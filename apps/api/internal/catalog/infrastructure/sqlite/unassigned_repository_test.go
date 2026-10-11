package sqlite_test

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

func TestUnassignedEntriesSurviveTheDatabase(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	repo := catalogsqlite.NewUnassignedRepository(conn)

	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	entry, err := domain.NewUnassignedEntry("batch-1", "Mission - Impossible", false, "batch-1", domain.RelationParts, now)
	if err != nil {
		t.Fatal(err)
	}
	entry.Files = []domain.UnassignedFile{
		{Source: "/staging/j1/a.mkv", Rel: "Season 01/a.mkv", Size: 10},
		{Source: "/staging/j1/a.srt", Rel: "Season 01/a.srt", Size: 2},
	}
	id, err := recordUnassigned(t.Context(), conn, entry)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == 0 {
		t.Fatal("the record returned no id")
	}

	got, found, err := repo.BySource(t.Context(), "batch-1")
	if err != nil || !found {
		t.Fatalf("BySource = %v, %v; want the entry", found, err)
	}
	if got.ID != id || got.Name != "Mission - Impossible" || got.Loose || got.BatchID != "batch-1" || got.Relation != domain.RelationParts ||
		got.Origin != domain.OriginUpload || !got.CreatedAt.Equal(now) {
		t.Errorf("entry = %+v; want its id, name, batch, relation, origin and time back", got)
	}
	if len(got.Files) != 2 || got.Files[0].Rel != "Season 01/a.mkv" || got.Files[1].Source != "/staging/j1/a.srt" {
		t.Errorf("files = %+v; want both, in order", got.Files)
	}

	extra := domain.UnassignedFile{Source: "/staging/j2/b.srt", Rel: "Season 01/b.srt", Size: 1}
	if err := addUnassignedFiles(t.Context(), conn, id, []domain.UnassignedFile{extra}); err != nil {
		t.Fatalf("add files: %v", err)
	}
	got, _, err = repo.BySource(t.Context(), "batch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 3 || got.Files[2].Source != extra.Source {
		t.Errorf("after adding a file: files = %+v", got.Files)
	}

	names, err := repo.Names(t.Context())
	if err != nil || len(names) != 1 || names[0] != "Mission - Impossible" {
		t.Errorf("Names = %v, %v; want the entry's name", names, err)
	}
	if _, found, err := repo.BySource(t.Context(), "unknown"); err != nil || found {
		t.Errorf("BySource of an unknown source = %v, %v; want not found", found, err)
	}
}

func TestUnassignedEntriesRefuseWhatTheSchemaDoes(t *testing.T) {
	t.Parallel()

	conn := openDB(t)
	repo := catalogsqlite.NewUnassignedRepository(conn)

	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	first, err := domain.NewUnassignedEntry("s1", "Movie", false, "b1", domain.RelationNone, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordUnassigned(t.Context(), conn, first); err != nil {
		t.Fatalf("record: %v", err)
	}

	sameSource, err := domain.NewUnassignedEntry("s1", "Other", false, "b1", domain.RelationNone, now)
	if err != nil {
		t.Fatal(err)
	}
	sameName, err := domain.NewUnassignedEntry("s2", "movie", false, "b1", domain.RelationNone, now)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		entry domain.UnassignedEntry
	}{
		{name: "a source recorded twice", entry: sameSource},
		{name: "a name taken in any letter case", entry: sameName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := recordUnassigned(t.Context(), conn, tt.entry); err == nil {
				t.Error("record accepted it")
			}
		})
	}
	if names, _ := repo.Names(t.Context()); len(names) != 1 {
		t.Errorf("names = %v; the refused entries must not be stored", names)
	}
}
