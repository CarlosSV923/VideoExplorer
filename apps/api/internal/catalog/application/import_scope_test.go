package application

import (
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestScopeImportKeepsOnlyTheAffectedTitleAndMergesWaitingReads(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	in := domain.ImportInput{
		Roots: []domain.DiskRoot{
			{Root: domain.ImportRoot{Name: "Movies", Kind: domain.KindMovie}, Entries: []domain.Folder{{Name: "Arrival (2016)"}, {Name: "Dune (2021)"}}},
			{Root: domain.ImportRoot{Name: "TV Shows", Kind: domain.KindSeries}, Entries: []domain.Folder{{Name: "Dark (2017)"}}},
		},
		Titles: []domain.RegisteredTitle{
			{Title: domain.LibraryTitle{ID: 1, Folder: "Movies/Arrival (2016)"}},
			{Title: domain.LibraryTitle{ID: 2, Folder: "Movies/Dune (2021)"}},
		},
		Seen: map[string]domain.Stamp{
			"movies/arrival (2016)/incoming.mkv": {Size: 1, ModTime: now},
			"movies/dune (2021)/waiting.mkv":     {Size: 2, ModTime: now},
		},
	}
	scoped := scopeImport(in, "Movies/Arrival (2016)")
	if len(scoped.Roots[0].Entries) != 1 || scoped.Roots[0].Entries[0].Name != "Arrival (2016)" {
		t.Fatalf("movie entries = %+v", scoped.Roots[0].Entries)
	}
	if len(scoped.Roots[1].Entries) != 0 || len(scoped.Titles) != 1 || scoped.Titles[0].Title.ID != 1 {
		t.Fatalf("scope includes unrelated titles: %+v %+v", scoped.Roots, scoped.Titles)
	}
	if len(scoped.Seen) != 1 {
		t.Fatalf("scoped seen reads = %+v", scoped.Seen)
	}
	if !scoped.SettledFolders["movies/arrival (2016)"] {
		t.Fatalf("affected folder was not marked settled: %+v", scoped.SettledFolders)
	}
	merged := mergeScopedWaiting(in.Seen, nil, "Movies/Arrival (2016)")
	if len(merged) != 1 || merged[0].Rel != "movies/dune (2021)/waiting.mkv" {
		t.Fatalf("merged waiting reads = %+v", merged)
	}
}
