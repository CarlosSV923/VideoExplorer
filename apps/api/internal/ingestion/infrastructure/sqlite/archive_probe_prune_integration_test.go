package sqlite_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

func TestPruneForgetsOnlyTheReadingsOfPathsThatAreNotKept(t *testing.T) {
	t.Parallel()

	const kept = "/library/_unassigned/kept.zip"
	const gone = "/library/_unassigned/gone.zip"
	tests := []struct {
		name     string
		keep     []string
		wantKept bool
		wantGone bool
	}{
		{name: "both still in _unassigned/", keep: []string{kept, gone}, wantKept: true, wantGone: true},
		{name: "one left: its reading is forgotten", keep: []string{kept}, wantKept: true, wantGone: false},
		{name: "nothing left in _unassigned/", keep: nil, wantKept: false, wantGone: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn, err := database.Open(t.Context(), t.TempDir())
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			repo := sqlite.NewArchiveProbeRepository(conn)
			for _, path := range []string{kept, gone} {
				if err := repo.Store(t.Context(), path, 10, 1, true); err != nil {
					t.Fatalf("Store: %v", err)
				}
			}

			if err := repo.Prune(t.Context(), tt.keep); err != nil {
				t.Fatalf("Prune: %v", err)
			}
			for _, c := range []struct {
				path string
				want bool
			}{{kept, tt.wantKept}, {gone, tt.wantGone}} {
				_, ok, err := repo.Lookup(t.Context(), c.path, 10, 1)
				if err != nil {
					t.Fatalf("Lookup: %v", err)
				}
				if ok != c.want {
					t.Errorf("reading of %s kept = %v, want %v", c.path, ok, c.want)
				}
			}
		})
	}
}
