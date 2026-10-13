package http

import (
	"context"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// Scans is what the API asks of the scans of the library (RF-28): their state, and the start of both on demand.
type Scans interface {
	// Status returns the state of both scans.
	Status() []application.ScanStatus
	// Trigger starts each scan that is not running and returns the state of both right after.
	Trigger() []application.ScanStatus
}

// ScanHandler implements the scan operations of httpapi.StrictServerInterface (RF-28).
type ScanHandler struct {
	scans Scans
}

// NewScanHandler builds the handler over the scans.
func NewScanHandler(scans Scans) *ScanHandler {
	return &ScanHandler{scans: scans}
}

// GetScans implements httpapi.StrictServerInterface (RF-28): the state of both scans.
func (h *ScanHandler) GetScans(context.Context, httpapi.GetScansRequestObject) (httpapi.GetScansResponseObject, error) {
	return httpapi.GetScans200JSONResponse(scanStatusesToAPI(h.scans.Status())), nil
}

// RunScans implements httpapi.StrictServerInterface («Escanear ahora», RF-28): both scans are started unless
// they run, and the answer is their state.
func (h *ScanHandler) RunScans(context.Context, httpapi.RunScansRequestObject) (httpapi.RunScansResponseObject, error) {
	return httpapi.RunScans202JSONResponse(scanStatusesToAPI(h.scans.Trigger())), nil
}

// scanStatusesToAPI turns the state of the scans into the contract's shape. lastFinishedAt is absent
// until the scan finishes for the first time.
func scanStatusesToAPI(list []application.ScanStatus) httpapi.ScanStatuses {
	out := httpapi.ScanStatuses{Scans: make([]httpapi.ScanStatus, 0, len(list))}
	for _, st := range list {
		v := httpapi.ScanStatus{Kind: httpapi.ScanKind(st.Kind), Running: st.Running}
		if !st.Finished.IsZero() {
			at := st.Finished
			v.LastFinishedAt = &at
		}
		out.Scans = append(out.Scans, v)
	}
	return out
}
