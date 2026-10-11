package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// memRepo keeps the entries in memory; failInsert makes the next insert fail.
type memRepo struct {
	entries    []domain.TrashEntry
	failInsert bool
}

func (m *memRepo) Insert(_ context.Context, e domain.TrashEntry) (domain.TrashEntry, error) {
	if m.failInsert {
		return domain.TrashEntry{}, errors.New("database is closed")
	}
	e.ID = int64(len(m.entries) + 1)
	m.entries = append(m.entries, e)
	return e, nil
}

func (m *memRepo) Entries(context.Context) ([]domain.TrashEntry, error) {
	return m.entries, nil
}

// library returns a library root with its trash, and a file to send there.
func library(t *testing.T) (root, source string) {
	t.Helper()
	root = t.TempDir()
	source = filepath.Join(root, ".videoexplorer", "staging", "job1")
	if err := os.MkdirAll(filepath.Dir(source), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("PK\x03\x04 the original archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, source
}

func TestPutMovesTheFileIntoTheTrashWithItsMetadata(t *testing.T) {
	t.Parallel()

	root, source := library(t)
	repo := &memRepo{}
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	trash := application.NewTrash(repo, libraryfs.New(root, filepath.Join(root, ".videoexplorer", "trash")), func() time.Time { return now })

	entry, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted,
		BatchID: "b1", RelativePath: "Folder/Movie.zip", Name: "Movie.zip", Source: source, Size: 25,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file is still at its source: %v", err)
	}
	moved := filepath.Join(root, ".videoexplorer", "trash", entry.Dir, "Movie.zip")
	data, err := os.ReadFile(moved)
	if err != nil || string(data) != "PK\x03\x04 the original archive" {
		t.Fatalf("trashed file = %q, %v", data, err)
	}
	if entry.ID == 0 || len(repo.entries) != 1 || repo.entries[0].Dir != entry.Dir {
		t.Fatalf("entry not recorded: %+v", repo.entries)
	}
	got := repo.entries[0]
	if got.BatchID != "b1" || got.RelativePath != "Folder/Movie.zip" {
		t.Errorf("provenance = %q %q, want the batch and the relative path it was dropped with", got.BatchID, got.RelativePath)
	}
	if got.Origin != domain.TrashOriginUpload || got.Reason != domain.TrashReasonExtracted ||
		!got.TrashedAt.Equal(now) || len(got.Files) != 1 || got.Files[0] != (domain.TrashFile{Name: "Movie.zip", Size: 25}) {
		t.Errorf("metadata = %+v", got)
	}
}

func TestPutRefusesWhatIsNotAValidEntryAndMovesNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  func(source string) application.PutRequest
	}{
		{name: "an unknown reason", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: "deleted", BatchID: "b1", RelativePath: "Movie.zip", Name: "Movie.zip", Source: s}
		}},
		{name: "no batch", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, RelativePath: "Movie.zip", Name: "Movie.zip", Source: s}
		}},
		{name: "a relative path that leaves the drop", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonFiltered, BatchID: "b1", RelativePath: "../Movie.zip", Name: "Movie.zip", Source: s}
		}},
		{name: "a file name that is a path", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "b1", RelativePath: "Movie.zip", Name: "../Movie.zip", Source: s}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, source := library(t)
			repo := &memRepo{}
			trash := application.NewTrash(repo, libraryfs.New(root, filepath.Join(root, ".videoexplorer", "trash")), nil)
			if _, err := trash.Put(t.Context(), tt.req(source)); !errors.Is(err, domain.ErrInvalidTrash) {
				t.Fatalf("err = %v, want ErrInvalidTrash", err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("the source was touched: %v", err)
			}
			if len(repo.entries) != 0 {
				t.Fatalf("an entry was recorded: %+v", repo.entries)
			}
		})
	}
}

func TestPutGivesTheFileBackWhenTheEntryCannotBeRecorded(t *testing.T) {
	t.Parallel()

	root, source := library(t)
	repo := &memRepo{failInsert: true}
	trash := application.NewTrash(repo, libraryfs.New(root, filepath.Join(root, ".videoexplorer", "trash")), nil)

	_, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "b1", RelativePath: "Folder/Movie.zip", Name: "Movie.zip", Source: source, Size: 25,
	})
	if err == nil {
		t.Fatal("Put succeeded without a recorded entry")
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "PK\x03\x04 the original archive" {
		t.Fatalf("the file did not go back to its source: %q, %v", data, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".videoexplorer", "trash")); len(entries) != 0 {
		t.Fatalf("the trash keeps a folder without its entry: %v", entries)
	}
}

func TestPutRecordsTheEntryInTheDatabase(t *testing.T) {
	t.Parallel()

	root, source := library(t)
	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	trash := application.NewTrash(catalogsqlite.NewTrashRepository(conn), libraryfs.New(root, filepath.Join(root, ".videoexplorer", "trash")), nil)
	if _, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "b1", RelativePath: "Folder/Movie.zip", Name: "Movie.zip", Source: source, Size: 25,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	entries, err := trash.Entries(t.Context())
	if err != nil || len(entries) != 1 || entries[0].Files[0].Name != "Movie.zip" {
		t.Fatalf("Entries = %+v, %v", entries, err)
	}
}

func TestPutKeepsFilteredFilesWithTheReasonAndTheirDropPath(t *testing.T) {
	t.Parallel()

	root, source := library(t)
	repo := &memRepo{}
	trash := application.NewTrash(repo, libraryfs.New(root, filepath.Join(root, ".videoexplorer", "trash")), nil)

	if _, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonFiltered,
		BatchID: "b7", RelativePath: "Movie/Samples/sample.mkv", Name: "sample.mkv", Source: source, Size: 25,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got := repo.entries[0]
	if got.Reason != domain.TrashReasonFiltered || got.BatchID != "b7" || got.RelativePath != "Movie/Samples/sample.mkv" {
		t.Errorf("entry = %+v, want the filtered reason with its batch and drop path", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".videoexplorer", "trash", got.Dir, "sample.mkv")); err != nil {
		t.Errorf("the discarded file is not in its trash entry: %v", err)
	}
}
