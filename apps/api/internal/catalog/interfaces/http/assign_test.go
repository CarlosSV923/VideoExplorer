package http_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// movieTMDB answers one TMDB movie (Iron Man, 2008), and no search results.
type movieTMDB struct{}

func (movieTMDB) Configured() bool { return true }

func (movieTMDB) SearchTitles(_ context.Context, kind domain.TitleKind, _ string, _ int) ([]application.TMDBTitle, error) {
	if kind != domain.KindMovie {
		return nil, nil
	}
	return []application.TMDBTitle{{ID: 1726, Title: "Iron Man", Year: 2008}}, nil
}

func (movieTMDB) Title(_ context.Context, kind domain.TitleKind, id int64) (application.TMDBTitle, error) {
	if kind != domain.KindMovie || id != 1726 {
		return application.TMDBTitle{}, application.ErrTMDBNotFound
	}
	return application.TMDBTitle{ID: 1726, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008}, nil
}

func (movieTMDB) Chapters(context.Context, int64, int, int, int) (application.TMDBChapters, error) {
	return application.TMDBChapters{}, application.ErrTMDBNotFound
}

// noNames reads no name: the handler tests choose every answer.
type noNames struct{}

func (noNames) Read(string, bool) application.NameReading { return application.NameReading{} }

// contentHash is the partial hash of a file, which is its content.
type contentHash struct{}

func (contentHash) PartialHash(_ context.Context, p string) (string, error) {
	b, err := os.ReadFile(p) //nolint:gosec // a file the test wrote
	if err != nil {
		return "", err
	}
	return "ph1:" + string(b), nil
}

// setup builds the assignment over a real database and a real library, with an entry of one loose
// video (its id is returned), and the handler over it.
func setup(t *testing.T) (*catalogshttp.Handler, int64) {
	t.Helper()
	ctx := t.Context()
	conn, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	root := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	files := libraryfs.New(root)
	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	journal := catalogsqlite.NewJournal(conn)
	repo := catalogsqlite.NewUnassignedRepository(conn)
	library := catalogsqlite.NewLibraryRepository(conn)
	engine := application.NewEngine(files, journal, clock, nil)

	const name = "Iron.Man.2008.mkv"
	disk := filepath.Join(root, domain.UnassignedDir, name)
	if err := os.WriteFile(disk, []byte("iron"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Settled: a file modified less than a minute before the clock is still being copied (RF-27).
	if err := os.Chtimes(disk, now.Add(-2*time.Minute), now.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var entryID int64
	if err := journal.Commit(ctx, "", func(rec application.Records) error {
		var err error
		entryID, err = rec.InsertUnassigned(ctx, domain.UnassignedEntry{
			Source: "batch-h", Name: name, Loose: true, BatchID: "b-h", Relation: domain.RelationNone,
			Origin: domain.OriginUpload, CreatedAt: now, Files: []domain.UnassignedFile{{Source: "staging/" + name, Size: 4}},
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewAssignments(application.AssignDeps{
		Unassigned: repo, Library: library, Files: files, Engine: engine,
		Finder: application.NewDuplicateFinder(library, files, contentHash{}, []string{".mkv"}),
		TMDB:   movieTMDB{}, Names: noNames{}, Roots: application.Roots{Movies: "Movies", Series: "TV Shows"},
		VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: clock,
		NewToken: func() string { return "tab-h" },
	})
	return catalogshttp.NewHandler(svc, nil), entryID
}

// token returns a pointer to a lock token, as a request carries it.
func token(s string) *httpapi.AssignLockToken { return &s }

func TestTheHandlerAnswersTheLockOfAnEntry(t *testing.T) {
	t.Parallel()
	h, id := setup(t)
	ctx := t.Context()

	taken, err := h.TakeAssignLock(ctx, httpapi.TakeAssignLockRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	lock, ok := taken.(httpapi.TakeAssignLock200JSONResponse)
	if !ok || lock.Token != "tab-h" {
		t.Fatalf("take = %#v, want the lock of this tab", taken)
	}

	other, err := h.TakeAssignLock(ctx, httpapi.TakeAssignLockRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	conflict, ok := other.(httpapi.TakeAssignLock409ApplicationProblemPlusJSONResponse)
	if !ok || conflict.Code != "assigning" || conflict.Status != 409 {
		t.Errorf("another tab = %#v, want 409 with code assigning (RF-26)", other)
	}

	missing, err := h.TakeAssignLock(ctx, httpapi.TakeAssignLockRequestObject{Id: id + 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := missing.(httpapi.TakeAssignLock404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("an unknown entry = %#v, want 404", missing)
	}

	released, err := h.ReleaseAssignLock(ctx, httpapi.ReleaseAssignLockRequestObject{Id: id, Params: httpapi.ReleaseAssignLockParams{XAssignLock: token("tab-h")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := released.(httpapi.ReleaseAssignLock204Response); !ok {
		t.Errorf("release = %#v, want 204", released)
	}
}

func TestTheHandlerRefusesWhatTheLockDoesNotAllow(t *testing.T) {
	t.Parallel()
	h, id := setup(t)
	ctx := t.Context()
	body := &httpapi.AssignRequest{Videos: []httpapi.AssignVideo{{File: "", Action: "place", Title: &httpapi.TitleChoice{Source: "tmdb", TmdbId: ptr(int64(1726)), Kind: kindPtr(domain.KindMovie)}}}, Subtitles: []httpapi.AssignSubtitle{}}

	sug, err := h.GetAssignSuggestions(ctx, httpapi.GetAssignSuggestionsRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := sug.(httpapi.GetAssignSuggestions409ApplicationProblemPlusJSONResponse); !ok || c.Code != "lock_lost" {
		t.Errorf("suggestions without the lock = %#v, want 409 lock_lost", sug)
	}

	plan, err := h.PlanAssign(ctx, httpapi.PlanAssignRequestObject{Id: id, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.(httpapi.PlanAssign409ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("plan without the lock = %#v, want 409", plan)
	}

	commit, err := h.CommitAssign(ctx, httpapi.CommitAssignRequestObject{Id: id, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := commit.(httpapi.CommitAssign409ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("commit without the lock = %#v, want 409", commit)
	}
}

func TestTheHandlerPlansAndCommitsThroughTheLock(t *testing.T) {
	t.Parallel()
	h, id := setup(t)
	ctx := t.Context()
	if _, err := h.TakeAssignLock(ctx, httpapi.TakeAssignLockRequestObject{Id: id}); err != nil {
		t.Fatal(err)
	}
	lock := httpapi.AssignLockToken("tab-h")

	// A request with a video left out is not valid: the plan says so, and the commit answers 422.
	empty := &httpapi.AssignRequest{Videos: []httpapi.AssignVideo{}, Subtitles: []httpapi.AssignSubtitle{}}
	plan, err := h.PlanAssign(ctx, httpapi.PlanAssignRequestObject{Id: id, Params: httpapi.PlanAssignParams{XAssignLock: &lock}, Body: empty})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := plan.(httpapi.PlanAssign200JSONResponse)
	if !ok || p.Valid || len(p.Errors) == 0 {
		t.Fatalf("plan of an empty request = %#v, want 200 and not valid", plan)
	}
	commit, err := h.CommitAssign(ctx, httpapi.CommitAssignRequestObject{Id: id, Params: httpapi.CommitAssignParams{XAssignLock: &lock}, Body: empty})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := commit.(httpapi.CommitAssign422JSONResponse); !ok {
		t.Errorf("commit of an invalid request = %#v, want 422", commit)
	}

	body := &httpapi.AssignRequest{
		Videos:    []httpapi.AssignVideo{{File: "", Action: "place", Title: &httpapi.TitleChoice{Source: "tmdb", TmdbId: ptr(int64(1726)), Kind: kindPtr(domain.KindMovie)}}},
		Subtitles: []httpapi.AssignSubtitle{},
	}
	plan, err = h.PlanAssign(ctx, httpapi.PlanAssignRequestObject{Id: id, Params: httpapi.PlanAssignParams{XAssignLock: &lock}, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := plan.(httpapi.PlanAssign200JSONResponse); !ok || !p.Valid || p.Videos[0].FinalPath == nil {
		t.Fatalf("plan = %#v, want a valid plan with a final path", plan)
	}
	commit, err = h.CommitAssign(ctx, httpapi.CommitAssignRequestObject{Id: id, Params: httpapi.CommitAssignParams{XAssignLock: &lock}, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := commit.(httpapi.CommitAssign200JSONResponse)
	if !ok || !res.EntryRemoved || len(res.Placed) != 1 {
		t.Fatalf("commit = %#v, want the entry removed (RF-26)", commit)
	}
}

func TestTheHandlerRejectsAnEmptyTitleSearch(t *testing.T) {
	t.Parallel()
	h, _ := setup(t)
	res, err := h.SearchTitles(t.Context(), httpapi.SearchTitlesRequestObject{Params: httpapi.SearchTitlesParams{Query: "  "}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.(httpapi.SearchTitles400ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("an empty search = %#v, want 400", res)
	}
	found, err := h.SearchTitles(t.Context(), httpapi.SearchTitlesRequestObject{Params: httpapi.SearchTitlesParams{Query: "iron"}})
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := found.(httpapi.SearchTitles200JSONResponse); !ok || !s.TmdbAvailable || len(s.Tmdb) != 1 || s.Tmdb[0].TmdbId == nil {
		t.Errorf("a search with TMDB = %#v, want the TMDB result", found)
	}
}

func ptr[T any](v T) *T { return &v }

func kindPtr(k domain.TitleKind) *httpapi.TitleKind {
	v := httpapi.TitleKind(k)
	return &v
}

func TestTheHandlerRefusesAnActionThatIsNotPlaceOrKeep(t *testing.T) {
	t.Parallel()
	h, id := setup(t)
	bad := &httpapi.AssignRequest{Videos: []httpapi.AssignVideo{{File: "", Action: "delete"}}, Subtitles: []httpapi.AssignSubtitle{}}
	plan, err := h.PlanAssign(t.Context(), httpapi.PlanAssignRequestObject{Id: id, Body: bad})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.(httpapi.PlanAssign400ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("an unknown action = %#v, want 400", plan)
	}
}
