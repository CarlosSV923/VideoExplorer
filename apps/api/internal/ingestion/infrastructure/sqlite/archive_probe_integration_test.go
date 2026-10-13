package sqlite_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

func TestTheArchiveProbesKeepTheReadingOfTheSizeAndTimeTheyWereMadeWith(t *testing.T) {
	t.Parallel()

	const path = "/library/_unassigned/movie.zip"
	tests := []struct {
		name string
		// store is what is kept first, and then, when it is set, what is kept again.
		store       []bool
		lookupSize  int64
		lookupMtime int64
		wantArchive bool
		wantOK      bool
	}{
		{name: "nothing kept for the path", store: nil, lookupSize: 10, lookupMtime: 1, wantOK: false},
		{name: "kept with the same size and time", store: []bool{true}, lookupSize: 10, lookupMtime: 1, wantArchive: true, wantOK: true},
		{name: "kept, but the size changed", store: []bool{true}, lookupSize: 11, lookupMtime: 1, wantOK: false},
		{name: "kept, but the time changed", store: []bool{true}, lookupSize: 10, lookupMtime: 2, wantOK: false},
		{name: "kept again: the last reading is the one kept", store: []bool{true, false}, lookupSize: 10, lookupMtime: 1, wantArchive: false, wantOK: true},
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
			for _, archive := range tt.store {
				if err := repo.Store(t.Context(), path, 10, 1, archive); err != nil {
					t.Fatalf("Store: %v", err)
				}
			}

			archive, ok, err := repo.Lookup(t.Context(), path, tt.lookupSize, tt.lookupMtime)
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			if ok != tt.wantOK || archive != tt.wantArchive {
				t.Fatalf("Lookup = (archive %v, ok %v), want (archive %v, ok %v)", archive, ok, tt.wantArchive, tt.wantOK)
			}
		})
	}
}
