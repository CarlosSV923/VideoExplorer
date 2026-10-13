package http_test

import (
	"context"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// fakeScans answers fixed states and counts the triggers.
type fakeScans struct {
	states   []application.ScanStatus
	triggers int
}

func (f *fakeScans) Status() []application.ScanStatus { return f.states }

func (f *fakeScans) Trigger() []application.ScanStatus {
	f.triggers++
	return f.states
}

func TestTheScansShowTheirStateWithTheTimesTheyFinished(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		states []application.ScanStatus
		want   []httpapi.ScanStatus
	}{
		{
			name: "neither scan finished yet: no time, not running",
			states: []application.ScanStatus{
				{Kind: domain.ScanUnassigned}, {Kind: domain.ScanLibrary},
			},
			want: []httpapi.ScanStatus{
				{Kind: httpapi.ScanKindUnassigned}, {Kind: httpapi.ScanKindLibrary},
			},
		},
		{
			name: "the unassigned scan is running after a previous end",
			states: []application.ScanStatus{
				{Kind: domain.ScanUnassigned, Finished: at, Running: true}, {Kind: domain.ScanLibrary},
			},
			want: []httpapi.ScanStatus{
				{Kind: httpapi.ScanKindUnassigned, LastFinishedAt: &at, Running: true},
				{Kind: httpapi.ScanKindLibrary},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := catalogshttp.NewScanHandler(&fakeScans{states: tt.states})
			resp, err := h.GetScans(context.Background(), httpapi.GetScansRequestObject{})
			if err != nil {
				t.Fatalf("GetScans: %v", err)
			}
			got, ok := resp.(httpapi.GetScans200JSONResponse)
			if !ok {
				t.Fatalf("answer = %T, want 200", resp)
			}
			assertScans(t, got.Scans, tt.want)
		})
	}
}

func TestEscanearAhoraStartsTheScansAndAnswersTheirStateWith202(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC)
	scans := &fakeScans{states: []application.ScanStatus{
		{Kind: domain.ScanUnassigned, Running: true}, {Kind: domain.ScanLibrary, Finished: at, Running: true},
	}}
	h := catalogshttp.NewScanHandler(scans)

	resp, err := h.RunScans(context.Background(), httpapi.RunScansRequestObject{})
	if err != nil {
		t.Fatalf("RunScans: %v", err)
	}
	got, ok := resp.(httpapi.RunScans202JSONResponse)
	if !ok {
		t.Fatalf("answer = %T, want 202", resp)
	}
	if scans.triggers != 1 {
		t.Fatalf("triggers = %d, want 1", scans.triggers)
	}
	assertScans(t, got.Scans, []httpapi.ScanStatus{
		{Kind: httpapi.ScanKindUnassigned, Running: true},
		{Kind: httpapi.ScanKindLibrary, LastFinishedAt: &at, Running: true},
	})
}

// assertScans compares the scans of an answer with the expected ones, in order.
func assertScans(t *testing.T, got, want []httpapi.ScanStatus) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("scans = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].Running != want[i].Running {
			t.Fatalf("scan %d = %+v, want %+v", i, got[i], want[i])
		}
		switch {
		case (got[i].LastFinishedAt == nil) != (want[i].LastFinishedAt == nil):
			t.Fatalf("scan %d last finished = %v, want %v", i, got[i].LastFinishedAt, want[i].LastFinishedAt)
		case want[i].LastFinishedAt != nil && !got[i].LastFinishedAt.Equal(*want[i].LastFinishedAt):
			t.Fatalf("scan %d last finished = %s, want %s", i, *got[i].LastFinishedAt, *want[i].LastFinishedAt)
		}
	}
}
