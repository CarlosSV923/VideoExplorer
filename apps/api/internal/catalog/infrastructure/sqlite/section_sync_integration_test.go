package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// fakeArrivals is the ingestion's taking of Samba copies, as the section sees it: its Adopt runs
// adopt, and it counts its calls.
type fakeArrivals struct {
	adopt func() error
	calls int
}

func (a *fakeArrivals) Adopt(context.Context) error {
	a.calls++
	return a.adopt()
}

func TestSyncTakesTheSettledCopiesBeforeItReadsTheSection(t *testing.T) {
	t.Parallel()

	taken := func(e *sectionEnv) string { return filepath.Join(e.root, domain.UnassignedDir, "taken.zip") }
	tests := []struct {
		name      string
		adopt     func(e *sectionEnv) func() error
		wantNames []string
	}{
		{
			name: "the copy the adoption takes is not listed",
			adopt: func(e *sectionEnv) func() error {
				return func() error { return os.Remove(taken(e)) }
			},
			wantNames: []string{"kept.zip"},
		},
		{
			name: "a failed adoption does not fail the reading, which lists the copy",
			adopt: func(*sectionEnv) func() error {
				return func() error { return errors.New("disk busy") }
			},
			wantNames: []string{"kept.zip", "taken.zip"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newSectionEnv(t)
			e.settled("taken.zip", "zip bytes")
			e.settled("kept.zip", "zip bytes")
			arrivals := &fakeArrivals{adopt: tt.adopt(e)}

			entries, err := e.section.Sync(t.Context(), arrivals)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if arrivals.calls != 1 {
				t.Fatalf("Adopt calls = %d, want 1", arrivals.calls)
			}
			var names []string
			for _, en := range entries {
				names = append(names, en.Name)
			}
			slices.Sort(names)
			if !slices.Equal(names, tt.wantNames) {
				t.Fatalf("listed %v, want %v", names, tt.wantNames)
			}
		})
	}
}
