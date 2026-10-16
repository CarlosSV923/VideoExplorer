package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

func TestTrashDeleteAndEmptyRemoveFilesAndSQLiteRowsThroughTheEngine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		empty bool
		want  int
	}{{name: "delete one entry", want: 1}, {name: "empty all entries", empty: true, want: 0}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conn := openDB(t)
			root := t.TempDir()
			for _, dir := range []string{".videoexplorer/trash", ".videoexplorer/ops", "source"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
			files := libraryfs.New(root)
			journal := catalogsqlite.NewJournal(conn)
			engine := application.NewEngine(files, journal, func() time.Time { return now }, nil)
			svc := application.NewTrash(catalogsqlite.NewTrashRepository(conn), files, engine, func() time.Time { return now })
			for n := range 2 {
				name := string(rune('a'+n)) + ".zip"
				source := filepath.Join(root, "source", name)
				if err := os.WriteFile(source, []byte(name), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.Put(t.Context(), application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "batch", RelativePath: name, Name: name, Source: source, Size: int64(len(name))}); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := svc.Entries(t.Context())
			if err != nil || len(entries) != 2 {
				t.Fatalf("entries before deletion: %+v %v", entries, err)
			}
			if tc.empty {
				err = svc.Empty(t.Context())
			} else {
				err = svc.Delete(t.Context(), entries[0].ID)
			}
			if err != nil {
				t.Fatalf("delete: %v", err)
			}
			entries, err = svc.Entries(t.Context())
			if err != nil || len(entries) != tc.want {
				t.Fatalf("entries after deletion: %+v %v, want %d", entries, err, tc.want)
			}
			for _, e := range entries {
				if _, err := os.Stat(files.TrashPath(e.Dir)); err != nil {
					t.Errorf("remaining trash folder %s missing: %v", e.Dir, err)
				}
			}
			trashDirs, err := os.ReadDir(files.TrashPath())
			if err != nil {
				t.Fatal(err)
			}
			if len(trashDirs) != tc.want {
				t.Fatalf("trash folders=%d want %d", len(trashDirs), tc.want)
			}
			pending, err := journal.Pending(t.Context())
			if err != nil || len(pending) != 0 {
				t.Fatalf("pending journal operations=%v, %v", pending, err)
			}
		})
	}
}
