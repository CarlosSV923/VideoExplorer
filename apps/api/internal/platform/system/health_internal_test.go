package system

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// A check that never answers fails with its own code after the deadline, and
// the checks that do answer still report their result.
func TestHandlerFailsACheckThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	h := &Handler{timeout: 100 * time.Millisecond, checks: []Check{
		{Name: "library", Code: httpapi.LibraryNotWritable, Run: func(context.Context) *Failure {
			<-release // a network share that hangs
			return nil
		}},
		{Name: "ffprobe", Code: httpapi.FfprobeMissing, Run: func(context.Context) *Failure { return nil }},
	}}

	resp, err := h.GetHealth(t.Context(), httpapi.GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	body := resp.(httpapi.GetHealth200JSONResponse).Body

	if body.Status != httpapi.Degraded {
		t.Fatalf("status = %q, want degraded", body.Status)
	}
	hung := body.Checks[0]
	if hung.Ok || hung.Code == nil || *hung.Code != httpapi.LibraryNotWritable {
		t.Fatalf("hung check = %+v, want a failure with code library_not_writable", hung)
	}
	if hung.Message == nil || !strings.Contains(*hung.Message, "did not finish within 100ms") {
		t.Errorf("hung check message = %v, want the timeout", hung.Message)
	}
	if !body.Checks[1].Ok {
		t.Errorf("ffprobe check = %+v, want it to pass", body.Checks[1])
	}
}
