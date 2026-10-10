// Package system implements process-level operations: the liveness probe and,
// from phase 1 on, the startup diagnostics (library, 7zz, ffprobe).
package system

import (
	"context"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// Handler implements the system operations of httpapi.StrictServerInterface.
type Handler struct{}

// NewHandler builds the handler. It has no diagnostics yet; they arrive with
// the startup checks of phase 1.
func NewHandler() *Handler {
	return &Handler{}
}

// GetHealth implements httpapi.StrictServerInterface.
func (h *Handler) GetHealth(context.Context, httpapi.GetHealthRequestObject) (httpapi.GetHealthResponseObject, error) {
	noStore := "no-store"
	return httpapi.GetHealth200JSONResponse{
		Body: httpapi.Health{
			Status: httpapi.Ok,
			Checks: []httpapi.Check{},
		},
		Headers: httpapi.GetHealth200ResponseHeaders{CacheControl: &noStore},
	}, nil
}
