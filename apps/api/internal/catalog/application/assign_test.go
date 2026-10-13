package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

// fakeTMDB answers the metadata the assignment asks for, from tables. down makes every call fail.
type fakeTMDB struct {
	configured bool
	down       bool
	movies     map[int64]application.TMDBTitle
	series     map[int64]application.TMDBTitle
	hits       map[string][]application.TMDBTitle
	// seasons holds, per series and season, the title of each chapter; a season missing here is unknown.
	seasons map[int64]map[int]map[int]string
}

func (f *fakeTMDB) Configured() bool { return f.configured }

func (f *fakeTMDB) SearchTitles(_ context.Context, kind domain.TitleKind, query string, _ int) ([]application.TMDBTitle, error) {
	if f.down {
		return nil, application.ErrTMDBUnavailable
	}
	return f.hits[string(kind)+"|"+strings.ToLower(query)], nil
}

func (f *fakeTMDB) Title(_ context.Context, kind domain.TitleKind, id int64) (application.TMDBTitle, error) {
	if f.down {
		return application.TMDBTitle{}, application.ErrTMDBUnavailable
	}
	table := f.movies
	if kind == domain.KindSeries {
		table = f.series
	}
	t, ok := table[id]
	if !ok {
		return application.TMDBTitle{}, application.ErrTMDBNotFound
	}
	return t, nil
}

func (f *fakeTMDB) Chapters(_ context.Context, seriesID int64, season, first, last int) (application.TMDBChapters, error) {
	if f.down {
		return application.TMDBChapters{}, application.ErrTMDBUnavailable
	}
	chapters, ok := f.seasons[seriesID][season]
	if !ok {
		return application.TMDBChapters{}, application.ErrTMDBNotFound
	}
	var titles []string
	var missing []int
	for n := first; n <= last; n++ {
		t, ok := chapters[n]
		if !ok {
			missing = append(missing, n)
			continue
		}
		titles = append(titles, t)
	}
	return application.TMDBChapters{Title: strings.Join(titles, " + "), Missing: missing}, nil
}

// contentHash is a partial hash that is the content itself: enough to tell the test files apart.
type contentHash struct{}

func (contentHash) PartialHash(_ context.Context, p string) (string, error) {
	b, err := os.ReadFile(p) //nolint:gosec // a test file the test wrote
	if err != nil {
		return "", err
	}
	return "ph1:" + string(b), nil
}

// assignEnv is a library on disk with a registry, an engine and the assignment, all in memory but
// the files.
type assignEnv struct {
	t       *testing.T
	root    string
	store   *memStore
	journal *memJournal
	clock   *time.Time
	tmdb    *fakeTMDB
	names   map[string]application.NameReading
	media   mediaByName
	svc     *application.Assignments
	tokens  int
}

func newAssignEnv(t *testing.T, files func(root string) application.Files) *assignEnv {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir, ".videoexplorer/staging"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	e := &assignEnv{
		t: t, root: root, store: &memStore{}, clock: new(time.Time),
		tmdb: &fakeTMDB{
			configured: true, movies: map[int64]application.TMDBTitle{}, series: map[int64]application.TMDBTitle{},
			hits: map[string][]application.TMDBTitle{}, seasons: map[int64]map[int]map[int]string{},
		},
		names: map[string]application.NameReading{}, media: mediaByName{},
	}
	*e.clock = time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	f := files(root)
	e.journal = &memJournal{store: e.store}
	engine := application.NewEngine(f, e.journal, func() time.Time { return *e.clock }, nil)
	finder := application.NewDuplicateFinder(e.store, f, contentHash{}, []string{".mkv", ".mp4"})
	e.svc = application.NewAssignments(application.AssignDeps{
		Unassigned: e.store, Library: e.store, Files: f, Engine: engine, Finder: finder,
		Media: e.media, TMDB: e.tmdb, Names: nameTable(e.names),
		Roots:     application.Roots{Movies: "Movies", Series: "TV Shows"},
		VideoExts: []string{".mkv", ".mp4"}, SubtitleExts: []string{".srt", ".sub"},
		Now: func() time.Time { return *e.clock },
		NewToken: func() string {
			e.tokens++
			return "tab-" + string(rune('a'+e.tokens))
		},
	})
	return e
}

// nameTable reads the names the test set for each path.
type nameTable map[string]application.NameReading

func (n nameTable) Read(rel string, _ bool) application.NameReading { return n[rel] }

// entry records an entry of No asignados with files (rel to content), as the ingestion leaves it, and
// writes them under _unassigned/. A loose entry is one file named after the entry.
func (e *assignEnv) entry(name string, relation string, loose bool, files map[string]string) int64 {
	e.t.Helper()
	entry := domain.UnassignedEntry{Source: "batch-" + name, Name: name, Loose: loose, BatchID: "b-" + name, Relation: relation, Origin: domain.OriginUpload, CreatedAt: *e.clock}
	for _, rel := range sortedKeys(files) {
		content := files[rel]
		disk := filepath.Join(e.root, domain.UnassignedDir, name, filepath.FromSlash(rel))
		if loose {
			disk = filepath.Join(e.root, domain.UnassignedDir, name)
		}
		writeFile(e.t, disk, content)
		entry.Files = append(entry.Files, domain.UnassignedFile{Source: "staging/" + name + "/" + rel, Rel: rel, Size: int64(len(content))})
	}
	id, err := e.store.InsertUnassigned(e.t.Context(), entry)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// sortedKeys returns the keys of files in order, so that an entry lists its files the same way each time.
func sortedKeys(files map[string]string) []string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// registerTitle puts a title and its files in the registry, the files written on disk too.
func (e *assignEnv) registerTitle(title domain.LibraryTitle, files map[string]string) int64 {
	e.t.Helper()
	id, err := e.store.InsertLibraryTitle(e.t.Context(), title)
	if err != nil {
		e.t.Fatal(err)
	}
	var rows []domain.LibraryFile
	for rel, content := range files {
		writeFile(e.t, filepath.Join(e.root, filepath.FromSlash(rel)), content)
		row, err := domain.NewVideo(title.Kind, domain.VideoSpec{Rel: rel, Size: int64(len(content))}, *e.clock)
		if err != nil {
			e.t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if err := e.store.InsertLibraryFiles(e.t.Context(), id, rows); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// lockAs takes the lock of an entry for a tab, as Asignar does when it opens.
func (e *assignEnv) lockAs(id int64) {
	const token = "tab-a"
	e.t.Helper()
	lock, err := e.svc.Take(e.t.Context(), id, token)
	if err != nil {
		e.t.Fatalf("take: %v", err)
	}
	if token != "" && lock.Token != token {
		e.t.Fatalf("token = %q, want %q", lock.Token, token)
	}
}

// tmdbMovie answers a TMDB movie and its search.
func (e *assignEnv) tmdbMovie(id int64, title, disc string, year int) {
	e.tmdb.movies[id] = application.TMDBTitle{ID: id, Title: title, DiscTitle: disc, Year: year}
}

func (e *assignEnv) tmdbSeries(id int64, title, disc string, year int) {
	e.tmdb.series[id] = application.TMDBTitle{ID: id, Title: title, DiscTitle: disc, Year: year}
}

func (e *assignEnv) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(rel)))
	return err == nil
}

func (e *assignEnv) read(rel string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

// movieTMDB chooses a TMDB movie for a video.
// registeredTitle chooses the title of the registry that the tests register first (id 1).
var registeredTitle = application.TitleChoice{Source: "library", LibraryID: 1}

func movieTMDB(id int64) application.TitleChoice {
	return application.TitleChoice{Source: "tmdb", TMDBID: id, Kind: domain.KindMovie}
}

func seriesTMDB(id int64) application.TitleChoice {
	return application.TitleChoice{Source: "tmdb", TMDBID: id, Kind: domain.KindSeries}
}

func intp(n int) *int { return &n }

func TestTakingTheLockKeepsTheEntryForOneTab(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	id := e.entry("Iron Man.mkv", domain.RelationNone, true, map[string]string{"": "iron"})
	ctx := t.Context()

	first, err := e.svc.Take(ctx, id, "")
	if err != nil {
		t.Fatalf("first tab takes the lock: %v", err)
	}
	if _, err := e.svc.Take(ctx, id, ""); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("second tab: err = %v, want ErrAssigning (RF-26)", err)
	}
	if _, err := e.svc.Suggest(ctx, id, "other"); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("another tab reads the suggestions: err = %v, want ErrAssigning", err)
	}
	if _, err := e.svc.Take(ctx, id, first.Token); err != nil {
		t.Errorf("the holder renews its lock: %v", err)
	}
	if err := e.svc.Release(ctx, id, "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Take(ctx, id, ""); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("a release by another tab frees nothing: err = %v", err)
	}

	*e.clock = e.clock.Add(domain.AssignLockTTL - time.Second)
	if _, err := e.svc.Suggest(ctx, id, first.Token); err != nil {
		t.Errorf("the lock holds until its time: %v", err)
	}
	*e.clock = e.clock.Add(time.Second)
	if _, err := e.svc.Suggest(ctx, id, first.Token); !errors.Is(err, application.ErrLockLost) {
		t.Errorf("an expired lock: err = %v, want ErrLockLost", err)
	}
	if _, err := e.svc.Take(ctx, id, ""); err != nil {
		t.Errorf("a free entry after the expiry: %v", err)
	}
}

func TestReleasingTheLockFreesTheEntry(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	id := e.entry("Iron Man.mkv", domain.RelationNone, true, map[string]string{"": "iron"})
	e.lockAs(id)
	if err := e.svc.Release(t.Context(), id, "tab-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Suggest(t.Context(), id, "tab-a"); !errors.Is(err, application.ErrLockLost) {
		t.Errorf("after release the holder has no lock: err = %v, want ErrLockLost", err)
	}
	if _, err := e.svc.Take(t.Context(), 99, ""); !errors.Is(err, application.ErrEntryNotFound) {
		t.Errorf("an unknown entry: err = %v, want ErrEntryNotFound", err)
	}
}

func TestSuggestionsReadTheNamesAndTheBestTMDBMatch(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbSeries(1396, "Breaking Bad", "Breaking Bad", 2008)
	e.tmdb.hits["series|breaking bad"] = []application.TMDBTitle{{ID: 1396, Title: "Breaking Bad", Year: 2008}}
	id := e.entry("Breaking.Bad.S01", domain.RelationEpisodes, false, map[string]string{
		"Breaking.Bad.S01E01.mkv":           "e1",
		"Breaking.Bad.S01E02.mkv":           "e2",
		"Breaking.Bad.S01E01.spa.srt":       "s1",
		"Breaking.Bad.S01E02.something.srt": "s2",
	})
	e.names["Breaking.Bad.S01/Breaking.Bad.S01E01.mkv"] = application.NameReading{Kind: domain.KindSeries, Title: "Breaking Bad", Year: 2008, Season: intp(1), Chapter: intp(1)}
	e.names["Breaking.Bad.S01/Breaking.Bad.S01E02.mkv"] = application.NameReading{Kind: domain.KindSeries, Title: "Breaking Bad", Year: 2008, Season: intp(1), Chapter: intp(2)}
	e.names["Breaking.Bad.S01/Breaking.Bad.S01E01.spa.srt"] = application.NameReading{Language: "es", Forced: false}
	e.names["Breaking.Bad.S01/Breaking.Bad.S01E02.something.srt"] = application.NameReading{}
	e.lockAs(id)

	s, err := e.svc.Suggest(t.Context(), id, "tab-a")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if !s.TMDBConfigured || s.Relation != string(domain.RelationEpisodes) {
		t.Fatalf("configured=%v relation=%q", s.TMDBConfigured, s.Relation)
	}
	if s.Title == nil || s.Title.TMDBID != 1396 || s.Title.Kind != domain.KindSeries {
		t.Errorf("entry title = %+v, want the series of the chapters (RF-03)", s.Title)
	}
	if len(s.Videos) != 2 || s.Videos[0].Match == nil || s.Videos[0].Reading.Chapter == nil || *s.Videos[0].Reading.Chapter != 1 {
		t.Fatalf("videos = %+v", s.Videos)
	}
	if len(s.Subtitles) != 2 {
		t.Fatalf("subtitles = %+v", s.Subtitles)
	}
	byFile := map[string]application.SubtitleSuggestion{}
	for _, sub := range s.Subtitles {
		byFile[sub.File] = sub
	}
	if got := byFile["Breaking.Bad.S01E01.spa.srt"]; got.Language != "es" || got.Video != "Breaking.Bad.S01E01.mkv" {
		t.Errorf("subtitle named after its video = %+v", got)
	}
	if got := byFile["Breaking.Bad.S01E02.something.srt"]; got.Language != application.DefaultSubtitleLanguage || got.Video != "Breaking.Bad.S01E02.mkv" {
		t.Errorf("subtitle with no language: %+v, want es-419 and its video (RF-23)", got)
	}
}

func TestSuggestionsWithoutTMDBKeepOnlyTheReading(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdb.configured = false
	id := e.entry("Heat.mkv", domain.RelationNone, true, map[string]string{"": "heat"})
	e.names["Heat.mkv"] = application.NameReading{Kind: domain.KindMovie, Title: "Heat", Year: 1995}
	e.lockAs(id)

	s, err := e.svc.Suggest(t.Context(), id, "tab-a")
	if err != nil {
		t.Fatal(err)
	}
	if s.TMDBConfigured || len(s.Videos) != 1 || s.Videos[0].Match != nil {
		t.Fatalf("without TMDB: %+v", s)
	}
	if s.Videos[0].Reading.Title != "Heat" || s.Videos[0].Reading.Year != 1995 {
		t.Errorf("the name is still read (§6): %+v", s.Videos[0].Reading)
	}
}

func TestSearchPutsTheLibraryTitlesFirst(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, nil)
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Le Fabuleux Destin d'Amélie Poulain", DiscTitle: "Le Fabuleux Destin d'Amélie Poulain",
		Year: 2001, Folder: "Movies/Amelie",
	}, nil)
	e.tmdb.hits["movie|iron"] = []application.TMDBTitle{{ID: 1726, Title: "Iron Man", Year: 2008}, {ID: 1788, Title: "Iron Man 2", Year: 2010}}

	res, err := e.svc.Search(t.Context(), "iron", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Library) != 1 || res.Library[0].Source != "library" || res.Library[0].Title != "Iron Man" {
		t.Errorf("library = %+v", res.Library)
	}
	if !res.TMDBAvailable || len(res.TMDB) != 2 {
		t.Fatalf("tmdb = %+v available=%v", res.TMDB, res.TMDBAvailable)
	}
	if res.TMDB[0].LibraryID == 0 || res.TMDB[1].LibraryID != 0 {
		t.Errorf("the TMDB title already in the library points to it: %+v", res.TMDB)
	}

	accents, err := e.svc.Search(t.Context(), "AMELIE poulain", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accents.Library) != 1 {
		t.Errorf("a search ignores case and accents (RF-42): %+v", accents.Library)
	}

	if _, err := e.svc.Search(t.Context(), "   ", 0); !errors.Is(err, application.ErrInvalidQuery) {
		t.Errorf("an empty search: err = %v, want ErrInvalidQuery", err)
	}

	e.tmdb.configured = false
	off, err := e.svc.Search(t.Context(), "iron", 0)
	if err != nil {
		t.Fatal(err)
	}
	if off.TMDBAvailable || len(off.TMDB) != 0 || len(off.Library) != 1 {
		t.Errorf("without TMDB only the library answers: %+v", off)
	}
}

// planOf runs a plan for an entry and fails the test on an error.
func planOf(t *testing.T, e *assignEnv, id int64, req application.Request) application.Plan {
	t.Helper()
	p, err := e.svc.Plan(t.Context(), id, "tab-a", req)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return p
}

func hasIssue(issues []application.FieldIssue, file, field, code string) bool {
	for _, i := range issues {
		if i.File == file && i.Field == field && i.Code == code {
			return true
		}
	}
	return false
}

func TestPlanShowsTheFinalPathsOfAMovieAndItsSubtitle(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	id := e.entry("Iron Man.mkv", domain.RelationNone, true, map[string]string{"": "iron"})
	e.lockAs(id)
	before := e.read(filepath.ToSlash(filepath.Join(domain.UnassignedDir, "Iron Man.mkv")))

	plan := planOf(t, e, id, application.Request{})
	if plan.Valid || !hasIssue(plan.Errors, "", "video", "missing") {
		t.Fatalf("a loose file with no answer is missing its decision: %+v", plan)
	}

	plan = planOf(t, e, id, application.Request{Videos: []application.VideoChoice{
		{File: "", Title: movieTMDB(1726)},
	}})
	if !plan.Valid {
		t.Fatalf("the movie is valid: %+v", plan.Errors)
	}
	if got := plan.Videos[0].FinalPath; got != "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv" {
		t.Errorf("final path = %q", got)
	}
	if e.read(filepath.ToSlash(filepath.Join(domain.UnassignedDir, "Iron Man.mkv"))) != before || e.exists("Movies/Iron Man (2008) [tmdbid-1726]") {
		t.Error("a plan changes nothing (RF-25)")
	}
}

func TestPlanNeedsEveryChoiceToBeValid(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	e.tmdbMovie(603, "The Matrix", "The Matrix", 1999)
	e.tmdbSeries(1396, "Breaking Bad", "Breaking Bad", 2008)
	id := e.entry("Mix", domain.RelationNone, false, map[string]string{"a.mkv": "a", "b.mkv": "b"})
	e.lockAs(id)

	tests := []struct {
		name  string
		req   application.Request
		file  string
		field string
		code  string
	}{
		{
			name: "a file that is not in the entry", req: application.Request{Videos: []application.VideoChoice{
				{File: "../secret.mkv", Keep: true}, {File: "a.mkv", Keep: true}, {File: "b.mkv", Keep: true},
			}},
			file: "../secret.mkv", field: "file", code: "unknown",
		},
		{name: "a video with no answer", req: application.Request{Videos: []application.VideoChoice{
			{File: "a.mkv", Keep: true},
		}}, file: "b.mkv", field: "video", code: "missing"},
		{
			name: "a movie title that is gone", req: application.Request{Videos: []application.VideoChoice{
				{File: "a.mkv", Title: application.TitleChoice{Source: "library", LibraryID: 7}}, {File: "b.mkv", Keep: true},
			}},
			file: "a.mkv", field: "title", code: "missing",
		},
		{
			name: "a movie with a season", req: application.Request{Videos: []application.VideoChoice{
				{File: "a.mkv", Title: movieTMDB(603), Season: intp(1)}, {File: "b.mkv", Keep: true},
			}},
			file: "a.mkv", field: "place", code: "kind_mismatch",
		},
		{
			name: "a chapter without its season", req: application.Request{Videos: []application.VideoChoice{
				{File: "a.mkv", Title: seriesTMDB(1396), Chapter: intp(1)}, {File: "b.mkv", Keep: true},
			}},
			file: "a.mkv", field: "season", code: "required",
		},
		{
			name: "a free title without a name", req: application.Request{Videos: []application.VideoChoice{
				{File: "a.mkv", Title: application.TitleChoice{Source: "free", Kind: domain.KindMovie}}, {File: "b.mkv", Keep: true},
			}},
			file: "a.mkv", field: "title", code: "required",
		},
		{
			name: "a TMDB title that is in the library already", req: application.Request{Videos: []application.VideoChoice{
				{File: "a.mkv", Title: movieTMDB(1726), Part: 1}, {File: "b.mkv", Keep: true},
			}},
			file: "a.mkv", field: "title", code: "in_library",
		},
	}
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, nil)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan := planOf(t, e, id, tc.req)
			if plan.Valid || !hasIssue(plan.Errors, tc.file, tc.field, tc.code) {
				t.Errorf("errors = %+v, want %s/%s/%s", plan.Errors, tc.file, tc.field, tc.code)
			}
		})
	}
}

func TestCommitMovesAMovieIntoTheLibraryAndRemovesTheEntry(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	id := e.entry("Iron Man 2008", domain.RelationNone, false, map[string]string{
		"Iron.Man.2008.1080p.mkv":    "iron",
		"Iron.Man.2008.1080p.es.srt": "subs",
	})
	e.lockAs(id)
	req := application.Request{
		Videos:    []application.VideoChoice{{File: "Iron.Man.2008.1080p.mkv", Title: movieTMDB(1726)}},
		Subtitles: []application.SubtitleChoice{{File: "Iron.Man.2008.1080p.es.srt", Video: "Iron.Man.2008.1080p.mkv", Language: "es-MX"}},
	}

	res, err := e.svc.Commit(t.Context(), id, "tab-a", req)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !res.EntryRemoved || len(res.Placed) != 1 {
		t.Errorf("result = %+v, want the entry gone (RF-26)", res)
	}
	video := "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"
	if e.read(video) != "iron" {
		t.Errorf("video content = %q", e.read(video))
	}
	if e.read("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].es-419.srt") != "subs" {
		t.Error("the subtitle went with its video, in es-419 (RF-23)")
	}
	if e.exists(filepath.ToSlash(filepath.Join(domain.UnassignedDir, "Iron Man 2008"))) {
		t.Error("the entry folder is removed when it is empty")
	}
	if _, err := e.svc.Take(t.Context(), id, ""); !errors.Is(err, application.ErrEntryNotFound) {
		t.Errorf("the entry is gone: err = %v", err)
	}
	if len(e.store.titles) != 1 || e.store.titles[0].Folder != "Movies/Iron Man (2008) [tmdbid-1726]" || len(e.store.files) != 2 {
		t.Errorf("registry = %+v files=%d", e.store.titles, len(e.store.files))
	}
}

func TestCommitPlacesChaptersUnderOneTitleAndSeason(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbSeries(1396, "Breaking Bad", "Breaking Bad", 2008)
	e.tmdb.seasons[1396] = map[int]map[int]string{1: {1: "Pilot", 2: "Cat's in the Bag"}}
	id := e.entry("Breaking.Bad.S01", domain.RelationEpisodes, false, map[string]string{
		"Breaking.Bad.S01E01.mkv": "e1",
		"Breaking.Bad.S01E02.mkv": "e2",
	})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{
		{File: "Breaking.Bad.S01E01.mkv", Title: seriesTMDB(1396), Season: intp(1), Chapter: intp(1)},
		{File: "Breaking.Bad.S01E02.mkv", Title: seriesTMDB(1396), Season: intp(1), Chapter: intp(2)},
	}}

	plan := planOf(t, e, id, req)
	if !plan.Valid {
		t.Fatalf("plan: %+v", plan.Errors)
	}
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	season := "TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/"
	if e.read(season+"Breaking Bad (2008) - S01E01 - Pilot.mkv") != "e1" || e.read(season+"Breaking Bad (2008) - S01E02 - Cat's in the Bag.mkv") != "e2" {
		t.Error("the chapters are named after their TMDB titles (spec §5)")
	}
	if len(e.store.titles) != 1 {
		t.Errorf("titles = %d, want one title for both chapters (RF-21)", len(e.store.titles))
	}
}

func TestPartsOfAMovieGetTheirNumbers(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(438631, "Dune", "Dune", 2021)
	id := e.entry("Dune", domain.RelationParts, false, map[string]string{"Dune.cd1.mkv": "p1", "Dune.cd2.mkv": "p2"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{
		{File: "Dune.cd1.mkv", Title: movieTMDB(438631), Part: 1},
		{File: "Dune.cd2.mkv", Title: movieTMDB(438631), Part: 2},
	}}
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	folder := "Movies/Dune (2021) [tmdbid-438631]/"
	if e.read(folder+"Dune (2021) [tmdbid-438631] - part1.mkv") != "p1" || e.read(folder+"Dune (2021) [tmdbid-438631] - part2.mkv") != "p2" {
		t.Error("each part takes its number (RF-22)")
	}
}

func TestPartsAndTheWholeMovieAreExclusive(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(438631, "Dune", "Dune", 2021)
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Dune", DiscTitle: "Dune", Year: 2021, TMDBID: 438631, IDTag: true,
		Folder: "Movies/Dune (2021) [tmdbid-438631]",
	}, map[string]string{"Movies/Dune (2021) [tmdbid-438631]/Dune (2021) [tmdbid-438631].mkv": "whole"})
	id := e.entry("Dune part", domain.RelationParts, false, map[string]string{"Dune.cd1.mkv": "p1"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{
		{File: "Dune.cd1.mkv", Title: application.TitleChoice{Source: "library", LibraryID: 1}, Part: 1, Duplicate: domain.DecisionKeepBoth},
	}}
	plan := planOf(t, e, id, req)
	if plan.Valid || !hasIssue(plan.Errors, "Dune.cd1.mkv", "duplicate", "not_allowed") {
		t.Errorf("keep both with parts: %+v", plan.Errors)
	}
	v := plan.Videos[0]
	if len(v.Duplicates) != 1 || len(v.Allowed) != 2 || v.Allowed[0] != domain.DecisionReplace || v.Allowed[1] != domain.DecisionSkip {
		t.Errorf("a part of a movie with the whole movie offers only Replace or Skip (RF-22): %+v", v)
	}
}

func TestDuplicatesNeedADecisionAndAnExactCopyIsSkipped(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, map[string]string{"Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv": "same"})
	id := e.entry("Iron Man copy.mkv", domain.RelationNone, true, map[string]string{"": "same"})
	e.lockAs(id)
	choice := application.VideoChoice{File: "", Title: application.TitleChoice{Source: "library", LibraryID: 1}}

	plan := planOf(t, e, id, application.Request{Videos: []application.VideoChoice{choice}})
	v := plan.Videos[0]
	if plan.Valid || !hasIssue(plan.Errors, "", "duplicate", "required") {
		t.Errorf("a duplicate needs an answer: %+v", plan.Errors)
	}
	if !v.ExactCopy || v.Proposal != domain.DecisionSkip {
		t.Errorf("an exact copy proposes Skip (RF-24): exact=%v proposal=%q", v.ExactCopy, v.Proposal)
	}

	choice.Duplicate = domain.DecisionSkip
	plan = planOf(t, e, id, application.Request{Videos: []application.VideoChoice{choice}})
	if !plan.Valid || plan.Videos[0].FinalPath != "" {
		t.Errorf("skip keeps the new file in No asignados: valid=%v errors=%+v", plan.Valid, plan.Errors)
	}
	res, err := e.svc.Commit(t.Context(), id, "tab-a", application.Request{Videos: []application.VideoChoice{choice}})
	if err != nil {
		t.Fatalf("Commit with Skip: %v", err)
	}
	if res.EntryRemoved || !e.exists("_unassigned/Iron Man copy.mkv") {
		t.Errorf("a skipped file stays in the entry: %+v", res)
	}
}

func TestReplaceSendsTheDuplicateToTheTrash(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	const old = "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, map[string]string{old: "old"})
	id := e.entry("Iron Man new.mkv", domain.RelationNone, true, map[string]string{"": "new"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{{File: "", Title: registeredTitle, Duplicate: domain.DecisionReplace}}}

	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if e.read(old) != "new" {
		t.Errorf("the new file takes the place: %q", e.read(old))
	}
	if len(e.store.trash) != 1 || e.store.trash[0].Reason != domain.TrashReasonReplaced {
		t.Errorf("the replaced file is in the trash with its reason (RF-24): %+v", e.store.trash)
	}
}

func TestKeepBothLabelsTheVersionsAndRecordsThem(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	const old = "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, map[string]string{old: "1080"})
	e.media[filepath.Base(old)] = domain.VersionMedia{Width: 1920, Height: 1080, Codec: "h264", Audio: []domain.AudioMedia{{Language: "eng"}}}
	id := e.entry("Iron.Man.4K.mkv", domain.RelationNone, true, map[string]string{"": "2160"})
	e.media["Iron.Man.4K.mkv"] = domain.VersionMedia{Width: 3840, Height: 2160, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "eng"}}}
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{{File: "", Title: registeredTitle, Duplicate: domain.DecisionKeepBoth}}}

	plan := planOf(t, e, id, req)
	if !plan.Valid || plan.Videos[0].Version != "2160p" {
		t.Fatalf("keep both proposes the label of the new version: valid=%v version=%q errors=%+v", plan.Valid, plan.Videos[0].Version, plan.Errors)
	}
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	oldLabelled := "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 1080p.mkv"
	newLabelled := "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - 2160p.mkv"
	if e.read(oldLabelled) != "1080" || e.read(newLabelled) != "2160" {
		t.Error("both versions keep their content under their labels (RF-24)")
	}
	if e.exists(old) {
		t.Error("the version without a label is renamed, not copied")
	}
	versions := map[string]string{}
	for _, f := range e.store.files {
		versions[f.Rel] = f.Version
	}
	if versions[oldLabelled] != "1080p" || versions[newLabelled] != "2160p" {
		t.Errorf("the registry knows the versions: %+v", versions)
	}
}

func TestACommitThatFailsMidWayLeavesTheEntryAsItWas(t *testing.T) {
	t.Parallel()
	var flaky *flakyFiles
	e := newAssignEnv(t, func(root string) application.Files {
		flaky = &flakyFiles{FS: libraryfs.New(root), failOn: "b.mkv"}
		return flaky
	})
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	id := e.entry("Two", domain.RelationParts, false, map[string]string{"a.mkv": "a", "b.mkv": "b"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{
		{File: "a.mkv", Title: movieTMDB(1726), Part: 1},
		{File: "b.mkv", Title: movieTMDB(1726), Part: 2},
	}}

	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); err == nil {
		t.Fatal("the commit must fail when a move fails")
	}
	if !e.exists(filepath.ToSlash(filepath.Join(domain.UnassignedDir, "Two", "a.mkv"))) || e.exists("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726] - part1.mkv") {
		t.Error("the move already done is undone (RNF-04)")
	}
	if len(e.store.titles) != 0 || e.store.count() != 1 {
		t.Errorf("no record survives a failed change: titles=%d entries=%d", len(e.store.titles), e.store.count())
	}
	s, err := e.svc.Suggest(t.Context(), id, "tab-a")
	if err != nil {
		t.Fatalf("the entry keeps its lock after a failure: %v", err)
	}
	if len(s.Videos) != 2 {
		t.Errorf("the entry keeps its files: %+v", s.Videos)
	}
}

func TestACommitOfAnInvalidRequestChangesNothing(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	id := e.entry("Two", domain.RelationNone, false, map[string]string{"a.mkv": "a", "b.mkv": "b"})
	e.lockAs(id)

	_, err := e.svc.Commit(t.Context(), id, "tab-a", application.Request{Videos: []application.VideoChoice{
		{File: "a.mkv", Title: movieTMDB(1726)},
	}})
	var invalid *application.PlanInvalidError
	if !errors.As(err, &invalid) || !errors.Is(err, application.ErrPlanInvalid) {
		t.Fatalf("err = %v, want a PlanInvalidError", err)
	}
	if invalid.Plan.Valid || !hasIssue(invalid.Plan.Errors, "b.mkv", "video", "missing") {
		t.Errorf("the plan says what is wrong: %+v", invalid.Plan.Errors)
	}
	if !e.exists("_unassigned/Two/a.mkv") || e.exists("Movies/Iron Man (2008) [tmdbid-1726]") || e.store.count() != 1 {
		t.Error("nothing changed")
	}
}

func TestKeptVideosStayInTheEntryAndTheEntryStays(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	id := e.entry("Iron Man pack", domain.RelationNone, false, map[string]string{"Iron.Man.mkv": "iron", "Extras/sample.mkv": "junk"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{
		{File: "Iron.Man.mkv", Title: movieTMDB(1726)},
		{File: "Extras/sample.mkv", Keep: true},
	}}

	res, err := e.svc.Commit(t.Context(), id, "tab-a", req)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.EntryRemoved {
		t.Error("the entry stays while something is left (RF-26)")
	}
	if !e.exists("_unassigned/Iron Man pack/Extras/sample.mkv") || e.exists("_unassigned/Iron Man pack/Iron.Man.mkv") {
		t.Error("the kept video stays where it was, the other moves")
	}
	if _, err := e.svc.Suggest(t.Context(), id, "tab-a"); !errors.Is(err, application.ErrLockLost) {
		t.Errorf("the commit released the lock: err = %v", err)
	}
	s, err := e.svc.Take(t.Context(), id, "")
	if err != nil || s.Token == "" {
		t.Errorf("the entry is free again: %v", err)
	}
}

func TestOnlyTheTabThatHoldsTheLockCanCommit(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	id := e.entry("Iron Man.mkv", domain.RelationNone, true, map[string]string{"": "iron"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{{File: "", Title: movieTMDB(1726)}}}

	if _, err := e.svc.Commit(t.Context(), id, "tab-b", req); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("another tab: err = %v, want ErrAssigning", err)
	}
	if _, err := e.svc.Plan(t.Context(), id, "", req); !errors.Is(err, application.ErrAssigning) {
		t.Errorf("a plan without the lock: err = %v, want ErrAssigning", err)
	}
	*e.clock = e.clock.Add(domain.AssignLockTTL)
	if _, err := e.svc.Commit(t.Context(), id, "tab-a", req); !errors.Is(err, application.ErrLockLost) {
		t.Errorf("an expired lock: err = %v, want ErrLockLost", err)
	}
}

func TestAnUnreadableLabelIsIncompleteAndNotInvented(t *testing.T) {
	t.Parallel()
	e := newAssignEnv(t, func(root string) application.Files { return libraryfs.New(root) })
	e.tmdbMovie(1726, "Iron Man", "Iron Man", 2008)
	const old = "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"
	e.registerTitle(domain.LibraryTitle{
		Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true,
		Folder: "Movies/Iron Man (2008) [tmdbid-1726]",
	}, map[string]string{old: "old"})
	id := e.entry("Iron Man unknown.mkv", domain.RelationNone, true, map[string]string{"": "new"})
	e.lockAs(id)
	req := application.Request{Videos: []application.VideoChoice{{File: "", Title: registeredTitle, Duplicate: domain.DecisionKeepBoth}}}

	plan := planOf(t, e, id, req)
	if plan.Valid || !hasIssue(plan.Errors, "", "version", "incomplete") {
		t.Fatalf("a label that cannot be read is incomplete until the user writes it: %+v", plan.Errors)
	}
}
