package sqlite_test

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
)

// oneMovieTMDB answers one movie, and no search, as a TMDB that knows Iron Man (2008) would.
type oneMovieTMDB struct{}

func (oneMovieTMDB) Configured() bool { return true }

func (oneMovieTMDB) SearchTitles(context.Context, domain.TitleKind, string, int) ([]application.TMDBTitle, error) {
	return nil, nil
}

func (oneMovieTMDB) Title(_ context.Context, kind domain.TitleKind, id int64) (application.TMDBTitle, error) {
	if kind != domain.KindMovie || id != 1726 {
		return application.TMDBTitle{}, application.ErrTMDBNotFound
	}
	return application.TMDBTitle{ID: 1726, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008}, nil
}

func (oneMovieTMDB) Chapters(context.Context, int64, int, int, int) (application.TMDBChapters, error) {
	return application.TMDBChapters{}, application.ErrTMDBNotFound
}

// contentPartialHash is a partial hash that is the content itself (enough for the duplicates here).
type contentPartialHash struct{}

func (contentPartialHash) PartialHash(_ context.Context, p string) (string, error) {
	b, err := os.ReadFile(p) //nolint:gosec // a file the test wrote
	if err != nil {
		return "", err
	}
	return "ph1:" + string(b), nil
}

// noNames reads no name: the test chooses every answer.
type noNames struct{}

func (noNames) Read(string, bool) application.NameReading { return application.NameReading{} }

// TestAnAssignmentOfAnEntryGoesThroughTheDatabase assigns an entry that the section put in place,
// with the real registry, journal and file system: the lock is kept in the database, the commit
// records the title and the file, and the entry disappears when nothing is left in it (RF-25, RF-26).
func TestAnAssignmentOfAnEntryGoesThroughTheDatabase(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	conn := openDB(t)
	root := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir, ".videoexplorer/staging"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	files := libraryfs.New(root)
	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	journal := catalogsqlite.NewJournal(conn)
	engine := application.NewEngine(files, journal, clock, nil)
	unassignedRepo := catalogsqlite.NewUnassignedRepository(conn)
	libraryRepo := catalogsqlite.NewLibraryRepository(conn)

	// The section receives a staged video, as the ingestion does (RF-08).
	staged := filepath.Join(root, ".videoexplorer", "staging", "Iron.Man.2008.1080p.mkv")
	if err := os.WriteFile(staged, []byte("iron man"), 0o600); err != nil {
		t.Fatal(err)
	}
	section := application.NewUnassigned(unassignedRepo, files, engine, clock)
	if _, err := section.Put(ctx, application.AsideRequest{
		Source: "batch-1", Name: "Iron.Man.2008.1080p.mkv", Loose: true, BatchID: "b1", Relation: domain.RelationNone,
		Files: []application.AsideFile{{Source: staged, Size: 8}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	entry, found, err := unassignedRepo.BySource(ctx, "batch-1")
	if err != nil || !found {
		t.Fatalf("the entry is recorded: found=%v err=%v", found, err)
	}

	newAssignments := func() *application.Assignments {
		return application.NewAssignments(application.AssignDeps{
			Unassigned: unassignedRepo, Library: libraryRepo, Files: files, Engine: engine,
			Finder: application.NewDuplicateFinder(libraryRepo, files, contentPartialHash{}, []string{".mkv"}),
			TMDB:   oneMovieTMDB{}, Names: noNames{}, Roots: application.Roots{Movies: "Movies", Series: "TV Shows"},
			VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: clock,
			NewToken: func() string { return "tab-a" },
		})
	}
	assign := newAssignments()

	lock, err := assign.Take(ctx, entry.ID, "")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	// The lock is in the database: another assignment object (as after a restart) sees it.
	if _, err := newAssignments().Take(ctx, entry.ID, "tab-b"); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("another tab, after a restart: err = %v, want ErrAssigning", err)
	}

	req := application.Request{Videos: []application.VideoChoice{
		{File: "", Title: application.TitleChoice{Source: "tmdb", TMDBID: 1726, Kind: domain.KindMovie}},
	}}
	plan, err := assign.Plan(ctx, entry.ID, lock.Token, req)
	if err != nil || !plan.Valid {
		t.Fatalf("Plan: valid=%v err=%v errors=%+v", plan.Valid, err, plan.Errors)
	}
	res, err := assign.Commit(ctx, entry.ID, lock.Token, req)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !res.EntryRemoved || len(res.Placed) != 1 {
		t.Errorf("result = %+v", res)
	}

	video := filepath.Join(root, "Movies", "Iron Man (2008) [tmdbid-1726]", "Iron Man (2008) [tmdbid-1726].mkv")
	if b, err := os.ReadFile(video); err != nil || string(b) != "iron man" {
		t.Errorf("the video is in the library: %q, %v", b, err)
	}
	if _, found, _ := unassignedRepo.BySource(ctx, "batch-1"); found {
		t.Error("the entry is gone from the registry")
	}
	titles, err := libraryRepo.Titles(ctx)
	if err != nil || len(titles) != 1 || titles[0].Folder != "Movies/Iron Man (2008) [tmdbid-1726]" || titles[0].TMDBID != 1726 {
		t.Errorf("registry titles = %+v, %v", titles, err)
	}
	pending, err := journal.Pending(ctx)
	if err != nil || len(pending) != 0 {
		t.Errorf("no change stays in the journal: %d, %v", len(pending), err)
	}
	if _, err := assign.Take(ctx, entry.ID, ""); !errors.Is(err, application.ErrEntryNotFound) {
		t.Errorf("the entry is not there any more: err = %v", err)
	}
}

// TestTheLockIsReleasedWhenTheTabLetsGo frees the entry the way the form does when it is cancelled, and
// then another tab can take it (RF-26).
func TestTheLockIsReleasedWhenTheTabLetsGo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	conn := openDB(t)
	repo := catalogsqlite.NewUnassignedRepository(conn)
	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	entryID, err := recordUnassigned(ctx, conn, domain.UnassignedEntry{
		Source: "batch-2", Name: "Heat.mkv", Loose: true, BatchID: "b2", Relation: domain.RelationNone,
		Origin: domain.OriginUpload, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assign := application.NewAssignments(application.AssignDeps{
		Unassigned: repo, Library: catalogsqlite.NewLibraryRepository(conn), Files: libraryfs.New(t.TempDir()),
		TMDB: oneMovieTMDB{}, Names: noNames{}, Now: func() time.Time { return now },
		NewToken: func() string { return "tab-a" },
	})
	lock, err := assign.Take(ctx, entryID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := assign.Release(ctx, entryID, "tab-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := assign.Take(ctx, entryID, "tab-b"); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("a release by another tab changes nothing: err = %v", err)
	}
	if err := assign.Release(ctx, entryID, lock.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := assign.Take(ctx, entryID, "tab-b"); err != nil {
		t.Errorf("the entry is free after the release: %v", err)
	}
}
