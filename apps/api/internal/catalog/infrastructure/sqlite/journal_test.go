package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// insertTrash records a trash entry as a change does: in the commit of the journal.
func insertTrash(ctx context.Context, conn *sql.DB, e domain.TrashEntry) (domain.TrashEntry, error) {
	var id int64
	err := catalogsqlite.NewJournal(conn).Commit(ctx, "", func(r application.Records) error {
		var err error
		id, err = r.InsertTrash(ctx, e)
		return err
	})
	e.ID = id
	return e, err
}

// recordUnassigned records an entry of No asignados with its files, in the commit of the journal.
func recordUnassigned(ctx context.Context, conn *sql.DB, e domain.UnassignedEntry) (int64, error) {
	var id int64
	err := catalogsqlite.NewJournal(conn).Commit(ctx, "", func(r application.Records) error {
		var err error
		id, err = r.InsertUnassigned(ctx, e)
		return err
	})
	return id, err
}

// addUnassignedFiles adds files to a recorded entry, in the commit of the journal.
func addUnassignedFiles(ctx context.Context, conn *sql.DB, id int64, files []domain.UnassignedFile) error {
	return catalogsqlite.NewJournal(conn).Commit(ctx, "", func(r application.Records) error {
		return r.AddUnassignedFiles(ctx, id, files)
	})
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestJournalKeepsAnOperationWithItsStepsUntilItIsForgotten(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	journal := catalogsqlite.NewJournal(conn)
	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	op := domain.Operation{ID: "op1", CreatedAt: now, Plan: domain.Plan{Kind: "trash.put", Source: "job1", Steps: []domain.Step{
		{Kind: domain.StepMkdir, To: "/lib/.videoexplorer/trash/x"},
		{Kind: domain.StepRename, From: "/lib/.videoexplorer/staging/a.zip", To: "/lib/.videoexplorer/trash/x/a.zip"},
		{Kind: domain.StepRemoveDir, From: "/lib/old"},
	}}}
	if err := journal.Save(t.Context(), op); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := journal.Advance(t.Context(), "op1", 2); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	pending, err := journal.Pending(t.Context())
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending = %+v, %v; want the operation", pending, err)
	}
	got := pending[0]
	if got.ID != "op1" || got.Started != 2 || got.Plan.Kind != "trash.put" || got.Plan.Source != "job1" || !got.CreatedAt.Equal(now) {
		t.Errorf("operation = %+v", got)
	}
	if !slices.Equal(got.Plan.Steps, op.Plan.Steps) {
		t.Errorf("steps = %+v, want %+v", got.Plan.Steps, op.Plan.Steps)
	}

	if err := journal.Forget(t.Context(), "op1"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if pending, _ := journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("pending after Forget = %+v", pending)
	}
}

func TestACommitRecordsAndForgetsTogether(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	journal := catalogsqlite.NewJournal(conn)
	op := domain.Operation{ID: "op1", CreatedAt: time.Now(), Plan: domain.Plan{Kind: "unassigned.put", Source: "s1", Steps: []domain.Step{}}}
	if err := journal.Save(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	entry, err := domain.NewUnassignedEntry("s1", "Movie", false, "b1", domain.RelationNone, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	entry.Files = []domain.UnassignedFile{{Source: "/staging/a.mkv", Rel: "a.mkv", Size: 3}}

	if err := journal.Commit(t.Context(), "op1", func(r application.Records) error {
		_, err := r.InsertUnassigned(t.Context(), entry)
		return err
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if pending, _ := journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("the commit kept its operation: %+v", pending)
	}
	if _, found, err := catalogsqlite.NewUnassignedRepository(conn).BySource(t.Context(), "s1"); err != nil || !found {
		t.Errorf("the record is missing after the commit: %v, %v", found, err)
	}
}

func TestAFailedCommitKeepsTheOperationAndRecordsNothing(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	journal := catalogsqlite.NewJournal(conn)
	if err := journal.Save(t.Context(), domain.Operation{ID: "op1", CreatedAt: time.Now(), Plan: domain.Plan{Kind: "trash.put", Source: "s"}}); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("disk full")
	err := journal.Commit(t.Context(), "op1", func(r application.Records) error {
		if _, err := r.InsertTrash(t.Context(), domain.TrashEntry{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, Dir: "zz", TrashedAt: time.Now()}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Commit = %v, want the record's error", err)
	}
	if pending, _ := journal.Pending(t.Context()); len(pending) != 1 {
		t.Errorf("pending = %d, want the operation kept", len(pending))
	}
	if entries, _ := catalogsqlite.NewTrashRepository(conn).Entries(t.Context()); len(entries) != 0 {
		t.Errorf("a record of the failed commit was kept: %+v", entries)
	}
}

// TestTrashPutRunsThroughTheJournal sends a file to the trash with the real database and
// file system: the entry is listed and the journal is empty afterwards.
func TestTrashPutRunsThroughTheJournal(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	root := t.TempDir()
	staged := filepath.Join(root, ".videoexplorer", "staging", "job1.zip")
	if err := os.MkdirAll(filepath.Dir(staged), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := libraryfs.New(root)
	engine := application.NewEngine(files, catalogsqlite.NewJournal(conn), nil, nil)
	trash := application.NewTrash(catalogsqlite.NewTrashRepository(conn), files, engine, nil)

	entry, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted,
		BatchID: "b1", RelativePath: "Movie.zip", Name: "Movie.zip", Source: staged, Size: 7,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if entry.ID == 0 {
		t.Error("the entry has no id")
	}
	if data, err := os.ReadFile(filepath.Join(root, ".videoexplorer", "trash", entry.Dir, "Movie.zip")); err != nil || string(data) != "archive" {
		t.Errorf("trashed file = %q, %v", data, err)
	}
	listed, err := trash.Entries(t.Context())
	if err != nil || len(listed) != 1 || listed[0].Files[0].Name != "Movie.zip" {
		t.Fatalf("Entries = %+v, %v", listed, err)
	}
	if pending, _ := catalogsqlite.NewJournal(conn).Pending(t.Context()); len(pending) != 0 {
		t.Errorf("the journal keeps %d operations", len(pending))
	}
}
