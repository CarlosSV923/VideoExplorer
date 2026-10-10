// Package system implements the process-level operations: the liveness probe
// and the startup diagnostics of RF-63 (writable library and data folders,
// 7zz and ffprobe). TMDB is not diagnosed here.
package system

import (
	"context"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// checkTimeout bounds one diagnostics round. A check that has not answered by
// then counts as failed, so a hung network share cannot hang /api/health.
const checkTimeout = 5 * time.Second

// noStore keeps /api answers out of caches (RNF-02).
const noStore = "no-store"

// Check is one named diagnostic. Run returns nil when the check passes; a
// failure says why. Code is the stable reason reported for a failure, and it
// is also the code of a check that does not answer in time.
type Check struct {
	Name string
	Code httpapi.CheckCode
	Run  func(ctx context.Context) *Failure
}

// Failure explains a failed check (RF-63). Message is technical, in English.
// Path, UID and GID are the data of the check; they are omitted when empty
// (Path) or nil (UID, GID).
type Failure struct {
	Message string
	Path    string
	UID     *int
	GID     *int
}

// Handler implements GetHealth by running its checks on every request.
type Handler struct {
	checks  []Check
	timeout time.Duration
}

// NewHandler builds the handler. The checks run concurrently and are reported
// in the order given.
func NewHandler(checks ...Check) *Handler {
	return &Handler{checks: checks, timeout: checkTimeout}
}

// GetHealth implements httpapi.StrictServerInterface. It always answers 200:
// `degraded` tells the UI that a check failed, and the process keeps running
// (RF-63).
func (h *Handler) GetHealth(ctx context.Context, _ httpapi.GetHealthRequestObject) (httpapi.GetHealthResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	checks := h.runAll(ctx)
	status := httpapi.Ok
	for _, c := range checks {
		if !c.Ok {
			status = httpapi.Degraded
		}
	}
	cacheControl := noStore
	return httpapi.GetHealth200JSONResponse{
		Body:    httpapi.Health{Status: status, Checks: checks},
		Headers: httpapi.GetHealth200ResponseHeaders{CacheControl: &cacheControl},
	}, nil
}

// runAll runs every check concurrently and returns their results in order. A
// check still running when the deadline ends fails with its code.
func (h *Handler) runAll(ctx context.Context) []httpapi.Check {
	answers := make([]chan *Failure, len(h.checks))
	for i, c := range h.checks {
		answers[i] = make(chan *Failure, 1) // buffered: a late answer never blocks its goroutine
		go func() { answers[i] <- c.Run(ctx) }()
	}

	out := make([]httpapi.Check, len(h.checks))
	for i, c := range h.checks {
		out[i] = result(c, h.await(ctx, answers[i]))
	}
	return out
}

// await returns the answer of one check, or a timeout failure when the
// deadline ends first. An answer that is already there wins over the deadline.
func (h *Handler) await(ctx context.Context, answer <-chan *Failure) *Failure {
	select {
	case failure := <-answer:
		return failure
	default:
	}
	select {
	case failure := <-answer:
		return failure
	case <-ctx.Done():
		return &Failure{Message: fmt.Sprintf("the check did not finish within %s", h.timeout)}
	}
}

// result converts the outcome of a check into its API form. A passing check
// carries only its name and ok; a failing one adds the code, the message and
// the data of the failure.
func result(c Check, failure *Failure) httpapi.Check {
	res := httpapi.Check{Name: c.Name, Ok: failure == nil}
	if failure == nil {
		return res
	}
	code := c.Code
	message := failure.Message
	res.Code = &code
	res.Message = &message
	if failure.Path != "" {
		path := failure.Path
		res.Path = &path
	}
	res.Uid, res.Gid = failure.UID, failure.GID
	return res
}
