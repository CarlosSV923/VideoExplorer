package sqlite_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

func TestTrashEntriesSurviveTheDatabase(t *testing.T) {
	t.Parallel()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	repo := catalogsqlite.NewTrashRepository(conn)

	first := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	entries := []domain.TrashEntry{
		{
			Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, Dir: "aa", TrashedAt: first,
			Files: []domain.TrashFile{{Name: "Movie.zip", Size: 10}},
		},
		{
			Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, Dir: "bb", TrashedAt: second,
			Files: []domain.TrashFile{{Name: "Show.7z", Size: 20}},
		},
	}
	for _, e := range entries {
		saved, err := repo.Insert(t.Context(), e)
		if err != nil {
			t.Fatalf("Insert(%s): %v", e.Dir, err)
		}
		if saved.ID == 0 {
			t.Errorf("Insert(%s) gave no id", e.Dir)
		}
	}

	got, err := repo.Entries(t.Context())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(got) != 2 || got[0].Dir != "aa" || got[1].Dir != "bb" {
		t.Fatalf("entries = %+v, want aa then bb", got)
	}
	if !got[0].TrashedAt.Equal(first) || got[0].Origin != domain.TrashOriginUpload || got[0].Reason != domain.TrashReasonExtracted {
		t.Errorf("first entry = %+v", got[0])
	}
	if len(got[1].Files) != 1 || got[1].Files[0] != (domain.TrashFile{Name: "Show.7z", Size: 20}) {
		t.Errorf("files of the second entry = %+v", got[1].Files)
	}
}

func TestTrashRejectsWhatTheSchemaDoes(t *testing.T) {
	t.Parallel()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	repo := catalogsqlite.NewTrashRepository(conn)

	base := domain.TrashEntry{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, Dir: "cc", TrashedAt: time.Now()}
	if _, err := repo.Insert(t.Context(), base); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	duplicate := base
	if _, err := repo.Insert(t.Context(), duplicate); err == nil {
		t.Error("two entries share a folder")
	}
	unknownOrigin := base
	unknownOrigin.Dir = "dd"
	unknownOrigin.Origin = "library"
	if _, err := repo.Insert(t.Context(), unknownOrigin); err == nil {
		t.Error("an origin the schema does not know was stored")
	}
}
