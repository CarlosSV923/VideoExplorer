// Package http adapts the ingestion use cases to the generated HTTP contract.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// heartbeat is how often an idle event stream sends a comment line, so that
// proxies do not close it (RF-10b).
const heartbeat = 25 * time.Second

// Subscriber gives access to the live job changes.
type Subscriber interface {
	Subscribe() (<-chan domain.JobView, func())
}

// Handler implements the batch and job operations of httpapi.StrictServerInterface.
// The uploads themselves go through the tus endpoint, not through here.
type Handler struct {
	svc       *application.Service
	sub       Subscriber
	shutdown  <-chan struct{}
	heartbeat time.Duration
	log       *slog.Logger
}

// NewHandler builds the handler over the ingestion service. sub delivers the
// job changes to the event stream. shutdown closes when the server stops, so
// open event streams end and do not delay the shutdown. log may be nil.
func NewHandler(svc *application.Service, sub Subscriber, shutdown <-chan struct{}, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Handler{svc: svc, sub: sub, shutdown: shutdown, heartbeat: heartbeat, log: log}
}

// CreateBatch implements httpapi.StrictServerInterface. It answers 400 for a
// relation outside RF-03 or a name that cannot be a folder name.
func (h *Handler) CreateBatch(ctx context.Context, req httpapi.CreateBatchRequestObject) (httpapi.CreateBatchResponseObject, error) {
	if req.Body == nil {
		return httpapi.CreateBatch400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The request needs a JSON body with a relation.")),
		}, nil
	}
	var name string
	if req.Body.Name != nil {
		name = *req.Body.Name
	}
	batch, err := h.svc.CreateBatch(ctx, name, domain.Relation(req.Body.Relation))
	if errors.Is(err, domain.ErrInvalidBatch) {
		return httpapi.CreateBatch400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The relation must be parts, episodes or none, and the name must be usable as a folder name.")),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	h.log.DebugContext(ctx, "batch created", "batch", batch.ID, "relation", batch.Relation)
	return httpapi.CreateBatch201JSONResponse(batchToAPI(*batch)), nil
}

// ListJobs implements httpapi.StrictServerInterface. It returns the 100 most
// recent jobs, newest first (RF-10b).
func (h *Handler) ListJobs(ctx context.Context, _ httpapi.ListJobsRequestObject) (httpapi.ListJobsResponseObject, error) {
	views, err := h.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(httpapi.ListJobs200JSONResponse, 0, len(views))
	for _, v := range views {
		out = append(out, jobToAPI(v))
	}
	return out, nil
}

// StreamJobEvents implements httpapi.StrictServerInterface.
func (h *Handler) StreamJobEvents(ctx context.Context, _ httpapi.StreamJobEventsRequestObject) (httpapi.StreamJobEventsResponseObject, error) {
	// Subscribe before reading the snapshot, so no change falls between the two.
	events, unsubscribe := h.sub.Subscribe()
	views, err := h.svc.List(ctx)
	if err != nil {
		unsubscribe()
		return nil, err
	}
	return eventStream{
		ctx:         ctx,
		shutdown:    h.shutdown,
		heartbeat:   h.heartbeat,
		events:      events,
		unsubscribe: unsubscribe,
		snapshot:    views,
		log:         h.log,
	}, nil
}

// eventStream writes server-sent events (RF-10b). It replaces the generated
// response type, which only flushes when the writer itself is an http.Flusher:
// the logging middleware wraps the writer, and http.ResponseController sees through it.
type eventStream struct {
	ctx         context.Context
	shutdown    <-chan struct{}
	heartbeat   time.Duration
	events      <-chan domain.JobView
	unsubscribe func()
	snapshot    []domain.JobView
	log         *slog.Logger
}

// VisitStreamJobEventsResponse implements httpapi.StreamJobEventsResponseObject.
// A client that went away, a server that is shutting down and a subscriber that
// fell behind all end the stream without an error: none of them is a failure of
// the server, and the client reconnects when it needs the events again.
func (s eventStream) VisitStreamJobEventsResponse(w http.ResponseWriter) error {
	defer s.unsubscribe()
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no") // disable buffering in nginx-style proxies
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	for _, v := range s.snapshot {
		if err := writeEvent(w, v); err != nil {
			return nil //nolint:nilerr // the client went away
		}
	}
	if err := rc.Flush(); err != nil {
		return nil //nolint:nilerr // the client went away
	}

	tick := time.NewTicker(s.heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-s.shutdown:
			return nil
		case <-tick.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return nil //nolint:nilerr // the client went away
			}
		case v, ok := <-s.events:
			if !ok {
				return nil
			}
			if err := writeEvent(w, v); err != nil {
				return nil //nolint:nilerr // the client went away
			}
		}
		if err := rc.Flush(); err != nil {
			return nil //nolint:nilerr // the client went away
		}
	}
}

// writeEvent writes one job as an event named job, with the whole job as data.
func writeEvent(w io.Writer, v domain.JobView) error {
	data, err := json.Marshal(jobToAPI(v))
	if err != nil {
		return fmt.Errorf("encode job event: %w", err)
	}
	_, err = fmt.Fprintf(w, "event: job\ndata: %s\n\n", data)
	return err
}

// jobToAPI maps a job with its batch to the HTTP representation (RF-10b).
func jobToAPI(v domain.JobView) httpapi.UploadJob {
	j := v.Job
	out := httpapi.UploadJob{
		Id:           string(j.ID),
		Batch:        batchToAPI(v.Batch),
		FileName:     j.FileName,
		RelativePath: j.RelativePath,
		Size:         j.Size,
		Received:     j.Received,
		Status:       httpapi.JobStatus(j.Status),
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
	if j.Error != "" {
		reason := httpapi.JobFailure(j.Error)
		out.Error = &reason
	}
	return out
}

func batchToAPI(b domain.Batch) httpapi.Batch {
	return httpapi.Batch{
		Id:        string(b.ID),
		Name:      b.Name,
		Relation:  httpapi.BatchRelation(b.Relation),
		CreatedAt: b.CreatedAt,
	}
}

func problem(status int, title, detail string) httpapi.Problem {
	return httpapi.Problem{Status: status, Title: title, Detail: &detail}
}
