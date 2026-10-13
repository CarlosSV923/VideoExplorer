package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// signalingStore is the scan store of the database, which tells each saved end on saved.
type signalingStore struct {
	*catalogsqlite.ScanRepository
	saved chan domain.ScanKind
}

func (s signalingStore) SetFinished(ctx context.Context, kind domain.ScanKind, at time.Time) error {
	if err := s.ScanRepository.SetFinished(ctx, kind, at); err != nil {
		return err
	}
	s.saved <- kind
	return nil
}

func TestTheScanTimesSurviveARestartOfTheDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	conn, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	at := time.Date(2026, time.October, 13, 10, 30, 15, 123456789, time.UTC)
	repo := catalogsqlite.NewScanRepository(conn)
	if err := repo.SetFinished(t.Context(), domain.ScanUnassigned, at); err != nil {
		t.Fatalf("SetFinished: %v", err)
	}
	if err := repo.SetFinished(t.Context(), domain.ScanUnassigned, at.Add(time.Minute)); err != nil {
		t.Fatalf("SetFinished again: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The process restarts: the database opens again and the times are still there, the last one kept.
	conn, err = database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open after restart: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	got, err := catalogsqlite.NewScanRepository(conn).Finished(t.Context())
	if err != nil {
		t.Fatalf("Finished: %v", err)
	}
	if len(got) != 1 || !got[domain.ScanUnassigned].Equal(at.Add(time.Minute)) {
		t.Fatalf("times after restart = %v, want only the unassigned scan at %s", got, at.Add(time.Minute))
	}
	if _, ok := got[domain.ScanLibrary]; ok {
		t.Fatal("a scan that never finished has a time")
	}
}

func TestTheScanOfUnassignedTakesTheSambaCopiesAndSavesItsTime(t *testing.T) {
	t.Parallel()

	e := newSectionEnv(t)
	taken := filepath.Join(e.root, domain.UnassignedDir, "taken.zip")
	e.settled("taken.zip", "zip bytes")
	e.settled("kept.zip", "zip bytes")
	arrivals := &fakeArrivals{adopt: func() error { return os.Remove(taken) }}

	store := signalingStore{ScanRepository: catalogsqlite.NewScanRepository(e.conn), saved: make(chan domain.ScanKind, 4)}
	scanner, err := application.NewScanner(t.Context(), application.ScanDeps{
		Section: e.section, Arrivals: arrivals, Store: store,
		UnassignedEvery: time.Minute, LibraryEvery: time.Minute,
		Now:   func() time.Time { return e.now },
		After: func(time.Duration) <-chan time.Time { return make(chan time.Time) },
	})
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}
	scanner.Trigger()
	for range 2 {
		select {
		case <-store.saved:
		case <-time.After(5 * time.Second):
			t.Fatal("the scans did not end")
		}
	}

	if arrivals.calls != 1 {
		t.Fatalf("the scan took the Samba copies %d times, want 1", arrivals.calls)
	}
	entries, err := e.section.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, en := range entries {
		names = append(names, en.Name)
	}
	if !slices.Equal(names, []string{"kept.zip"}) {
		t.Fatalf("the section lists %v after the scan, want only kept.zip", names)
	}
	saved, err := catalogsqlite.NewScanRepository(e.conn).Finished(t.Context())
	if err != nil {
		t.Fatalf("Finished: %v", err)
	}
	if !saved[domain.ScanUnassigned].Equal(e.now) {
		t.Fatalf("saved unassigned time = %s, want %s", saved[domain.ScanUnassigned], e.now)
	}
}
