package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

// editTMDB answers the titles and the chapters the edits ask for: Iron Man (2008) and Breaking Bad (2008),
// whose chapters are named as their episode numbers. Season 9 is one TMDB does not know.
type editTMDB struct{}

func (editTMDB) Configured() bool { return true }

func (editTMDB) SearchTitles(context.Context, domain.TitleKind, string, int) ([]application.TMDBTitle, error) {
	return nil, nil
}

func (editTMDB) Title(_ context.Context, kind domain.TitleKind, id int64) (application.TMDBTitle, error) {
	switch {
	case kind == domain.KindMovie && id == 1726:
		return application.TMDBTitle{ID: 1726, Title: "Iron Man", DiscTitle: "Iron Man", Year: 2008, PosterFile: "poster.jpg"}, nil
	case kind == domain.KindSeries && id == 1396:
		return application.TMDBTitle{ID: 1396, Title: "Breaking Bad", DiscTitle: "Breaking Bad", Year: 2008}, nil
	}
	return application.TMDBTitle{}, application.ErrTMDBNotFound
}

func (editTMDB) Chapters(_ context.Context, seriesID int64, season, first, last int) (application.TMDBChapters, error) {
	if seriesID != 1396 || season == 9 {
		return application.TMDBChapters{}, application.ErrTMDBNotFound
	}
	out := application.TMDBChapters{Title: "Chapter"}
	if first == last {
		out.Title = "Episode " + string(rune('0'+first))
	}
	if first > 3 {
		out.Missing = []int{first}
	}
	return out, nil
}

// editEnv is a library in a temporary folder, with the real registry, journal and file system (RF-44).
type editEnv struct {
	t     *testing.T
	ctx   context.Context
	conn  *sql.DB
	root  string
	repo  *catalogsqlite.LibraryRepository
	edits *application.Edits
	now   time.Time
}

// newEditEnv starts an empty library. wrap, when not nil, wraps the file system (to make one step fail).
func newEditEnv(t *testing.T, wrap func(application.Files) application.Files) *editEnv {
	t.Helper()
	ctx := t.Context()
	conn := openDB(t)
	root := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir, ".videoexplorer/staging"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	var files application.Files = libraryfs.New(root)
	if wrap != nil {
		files = wrap(files)
	}
	now := libraryNow
	clock := func() time.Time { return now }
	repo := catalogsqlite.NewLibraryRepository(conn)
	engine := application.NewEngine(files, catalogsqlite.NewJournal(conn), clock, nil)
	edits := application.NewEdits(application.EditDeps{
		Library: repo, Files: files, Engine: engine,
		Finder: application.NewDuplicateFinder(repo, files, contentPartialHash{}, []string{".mkv"}),
		TMDB:   editTMDB{}, Roots: application.Roots{Movies: "Movies", Series: "TV Shows"},
		SubtitleExts: []string{".srt"}, Now: clock,
	})
	return &editEnv{t: t, ctx: ctx, conn: conn, root: root, repo: repo, edits: edits, now: now}
}

// write puts a file of the library on the disk, with content, and returns its library path.
func (e *editEnv) write(rel, content string) string {
	e.t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return rel
}

// exists reports whether a library path is on the disk.
func (e *editEnv) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(rel)))
	return err == nil
}

// rows returns the registry's files of a title by path, live and trashed ones alike.
func (e *editEnv) rows(titleID int64) map[string]domain.LibraryFile {
	e.t.Helper()
	files, err := e.repo.Files(e.ctx, titleID)
	if err != nil {
		e.t.Fatalf("Files: %v", err)
	}
	out := map[string]domain.LibraryFile{}
	for _, f := range files {
		out[f.Rel] = f
	}
	return out
}

// titleOf returns the registry title whose folder is folder.
func (e *editEnv) titleOf(folder string) (domain.LibraryTitle, bool) {
	e.t.Helper()
	t, ok, err := e.repo.TitleByFolder(e.ctx, folder)
	if err != nil {
		e.t.Fatalf("TitleByFolder: %v", err)
	}
	return t, ok
}

// seedMovie records a free movie with the files given (the videos first, then their subtitles).
func (e *editEnv) seedMovie(folder string, tmdbID int64, idTag bool, files ...domain.LibraryFile) int64 {
	e.t.Helper()
	return recordTitle(e.t, e.conn, mustTitle(e.t, domain.KindMovie, "Iron Man", folder, tmdbID, idTag), files...)
}

// freeVideo is a video of a free movie or of a free series, as the registry keeps it.
func (e *editEnv) video(kind domain.TitleKind, rel string, place domain.Place, version string) domain.LibraryFile {
	e.t.Helper()
	return mustVideo(e.t, kind, domain.VideoSpec{Rel: rel, Place: place, Version: version, Size: 4})
}

// movieChoices is the request that keeps every file as it is, with the title the files get.
func movieChoices(files map[string]domain.LibraryFile, title application.TitleChoice) application.EditRequest {
	req := application.EditRequest{Title: title}
	for _, f := range files {
		if f.Kind == domain.FileVideo {
			req.Videos = append(req.Videos, application.EditVideo{FileID: f.ID, Part: f.Place.Part, Version: f.Version})
		} else {
			req.Subtitles = append(req.Subtitles, application.EditSubtitle{FileID: f.ID, Language: f.Language, Forced: f.Forced, SDH: f.SDH})
		}
	}
	return req
}

// plan returns the preview of an edit; the form sends its revision back with the commit (RF-25).
func (e *editEnv) plan(titleID int64, req application.EditRequest) application.EditPlan {
	e.t.Helper()
	p, err := e.edits.Plan(e.ctx, titleID, req)
	if err != nil {
		e.t.Fatalf("Plan: %v", err)
	}
	return p
}

// commit stores an edit and fails the test on an error.
func (e *editEnv) commit(titleID int64, req application.EditRequest) application.EditResult {
	e.t.Helper()
	res, err := e.edits.Commit(e.ctx, titleID, req)
	if err != nil {
		e.t.Fatalf("Commit: %v", err)
	}
	return res
}

// assertNothingJournaled checks that no change of the library is left in the journal.
func (e *editEnv) assertNothingJournaled() {
	e.t.Helper()
	pending, err := catalogsqlite.NewJournal(e.conn).Pending(e.ctx)
	if err != nil || len(pending) != 0 {
		e.t.Errorf("journal: %d operations, %v", len(pending), err)
	}
}

const ironFolder = "Movies/Iron Man (2008)"

// TestLinkingAFreeMovieRenamesItsFolderAndFilesAndTagsIt links a free movie to TMDB: its folder and its
// files take the title of §5 and the tag [tmdbid-ID] (RF-44, RF-45).
func TestLinkingAFreeMovieRenamesItsFolderAndFilesAndTagsIt(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	sub := e.write(ironFolder+"/Iron Man (2008).es.srt", "sub")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, video, domain.Place{}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})

	p := e.plan(titleID, req)
	if !p.Valid || p.Folder != "Movies/Iron Man (2008) [tmdbid-1726]" {
		t.Fatalf("plan: valid=%v folder=%q errors=%+v", p.Valid, p.Folder, p.Errors)
	}
	if p.Videos[0].FinalPath != "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv" {
		t.Errorf("the final path of the video: %q", p.Videos[0].FinalPath)
	}
	req.Revision = p.Revision

	res := e.commit(titleID, req)
	if !res.Changed || res.TitleID != titleID {
		t.Errorf("result = %+v", res)
	}
	newVideo := "Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv"
	if !e.exists(newVideo) || !e.exists("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].es.srt") {
		t.Error("the video and its subtitle are at their new names")
	}
	if e.exists(ironFolder) {
		t.Error("the old folder is gone")
	}
	title, ok := e.titleOf("Movies/Iron Man (2008) [tmdbid-1726]")
	if !ok || title.TMDBID != 1726 || !title.IDTag || title.Title != "Iron Man" {
		t.Errorf("the registry title = %+v, found %v", title, ok)
	}
	after := e.rows(titleID)
	if _, ok := after[newVideo]; !ok || len(after) != 2 {
		t.Errorf("the registry files = %+v", after)
	}
	e.assertNothingJournaled()
}

// TestChoosingTheCurrentMatchKeepsEveryExistingName covers imported and canonical files. Selecting the same
// TMDB match is not a naming change, so the edit plan must report changed=false even when RF-29 preserved an
// [imdbid-] filename that differs from the canonical Jellyfin name (RF-44, RF-45).
func TestChoosingTheCurrentMatchKeepsEveryExistingName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		folder string
		file   string
		idTag  bool
	}{
		{
			name:   "imported filename",
			folder: ironFolder,
			file:   ironFolder + "/Iron Man (2008) [imdbid-tt18412256].mkv",
		},
		{
			name:   "canonical filename",
			folder: ironFolder + " [tmdbid-1726]",
			file:   ironFolder + " [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv",
			idTag:  true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEditEnv(t, nil)
			e.write(tc.file, "iron")
			titleID := e.seedMovie(tc.folder, 1726, tc.idTag, e.video(domain.KindMovie, tc.file, domain.Place{}, ""))
			req := movieChoices(e.rows(titleID), application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})

			p := e.plan(titleID, req)
			if !p.Valid || p.Videos[0].Changed || p.Videos[0].FinalPath != tc.file {
				t.Fatalf("same match changed the imported name: valid=%v changed=%v final=%q errors=%+v", p.Valid, p.Videos[0].Changed, p.Videos[0].FinalPath, p.Errors)
			}
		})
	}
}

// TestAStaleRevisionIsRefusedAsPlanChanged checks that a commit does not run on a title that changed since
// the plan was made (RF-25: plan_changed).
func TestAStaleRevisionIsRefusedAsPlanChanged(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := e.seedMovie(ironFolder, 0, false, e.video(domain.KindMovie, video, domain.Place{}, ""))
	req := movieChoices(e.rows(titleID), application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})
	req.Revision = e.plan(titleID, req).Revision

	// Another edit changes the title first: its version label.
	other := movieChoices(e.rows(titleID), application.TitleChoice{Source: "free", Kind: domain.KindMovie, Title: "Iron Man", Year: 2008})
	other.Revision = e.plan(titleID, other).Revision
	for i := range other.Videos {
		other.Videos[i].Version = "1080p"
	}
	if _, err := e.edits.Commit(e.ctx, titleID, other); err != nil {
		t.Fatalf("the other edit: %v", err)
	}
	if _, err := e.edits.Commit(e.ctx, titleID, req); !errors.Is(err, application.ErrPlanChanged) {
		t.Errorf("a commit with the old revision: err = %v, want ErrPlanChanged", err)
	}
}

// TestChangingTheVersionLabelRenamesOnlyThatFile keeps the name of the file that is not edited (RF-44).
func TestChangingTheVersionLabelRenamesOnlyThatFile(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	whole := e.write(ironFolder+"/Iron Man (2008).mkv", "whole")
	version := e.write(ironFolder+"/Iron Man (2008) - 1080p.mkv", "version")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, whole, domain.Place{}, ""),
		e.video(domain.KindMovie, version, domain.Place{}, "1080p"))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "library", LibraryID: titleID})
	for i := range req.Videos {
		if rows[version].ID == req.Videos[i].FileID {
			req.Videos[i].Version = "2160p"
		}
	}
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("plan: %+v", p.Errors)
	}
	req.Revision = p.Revision
	e.commit(titleID, req)
	if !e.exists(whole) {
		t.Error("the file that is not edited keeps its name")
	}
	if e.exists(version) || !e.exists(ironFolder+"/Iron Man (2008) - 2160p.mkv") {
		t.Error("the edited version takes its new label")
	}
}

// TestChangingAPartMovesTheFileToItsNumber renames a part of a movie to another number (RF-22, RF-44).
func TestChangingAPartMovesTheFileToItsNumber(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	p1 := e.write(ironFolder+"/Iron Man (2008) - part1.mkv", "one")
	p2 := e.write(ironFolder+"/Iron Man (2008) - part2.mkv", "two")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, p1, domain.Place{Part: 1}, ""),
		e.video(domain.KindMovie, p2, domain.Place{Part: 2}, ""))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "library", LibraryID: titleID})
	for i := range req.Videos {
		if rows[p1].ID == req.Videos[i].FileID {
			req.Videos[i].Part = 3
		}
	}
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("plan: %+v", p.Errors)
	}
	req.Revision = p.Revision
	e.commit(titleID, req)
	if e.exists(p1) || !e.exists(ironFolder+"/Iron Man (2008) - part3.mkv") || !e.exists(p2) {
		t.Error("the first part is now part 3; the second keeps its name")
	}
}

// TestAPartThatClashesWithAnotherPartIsAnIssue checks that two parts cannot take one number (RF-22).
func TestAPartThatClashesWithAnotherPartIsAnIssue(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	p1 := e.write(ironFolder+"/Iron Man (2008) - part1.mkv", "one")
	p2 := e.write(ironFolder+"/Iron Man (2008) - part2.mkv", "two")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, p1, domain.Place{Part: 1}, ""),
		e.video(domain.KindMovie, p2, domain.Place{Part: 2}, ""))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "library", LibraryID: titleID})
	for i := range req.Videos {
		if rows[p1].ID == req.Videos[i].FileID {
			req.Videos[i].Part = 2
		}
	}
	p := e.plan(titleID, req)
	if p.Valid {
		t.Fatal("two parts with one number are refused")
	}
	if _, err := e.edits.Commit(e.ctx, titleID, withRevision(req, p.Revision)); !errors.Is(err, application.ErrPlanInvalid) {
		t.Errorf("commit of an invalid plan: err = %v, want ErrPlanInvalid", err)
	}
	if !e.exists(p1) {
		t.Error("nothing moved")
	}
}

// withRevision returns the request with the revision set.
func withRevision(req application.EditRequest, revision string) application.EditRequest {
	req.Revision = revision
	return req
}

// lizzieFolder is a free series with two chapters, the second one in season 1 (chapter titles from the names).
const lizzieFolder = "TV Shows/Lizzie (2008)"

// seedSeries records a free series with the chapters of season 1, each video with its title in its name.
func (e *editEnv) seedSeries() (int64, map[string]domain.LibraryFile) {
	e.t.Helper()
	pilot := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv", "pilot")
	trial := e.write(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Trial.mkv", "trial")
	id := recordTitle(e.t, e.conn, mustTitle(e.t, domain.KindSeries, "Lizzie", lizzieFolder, 0, false),
		e.video(domain.KindSeries, pilot, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""),
		e.video(domain.KindSeries, trial, domain.Place{Episode: true, Season: 1, First: 2, Last: 2}, ""))
	return id, e.rows(id)
}

// chapterChoices returns the request that keeps every chapter where it is, with the chapter title each file reads.
func chapterChoices(rows map[string]domain.LibraryFile, titleID int64) application.EditRequest {
	req := application.EditRequest{Title: application.TitleChoice{Source: "library", LibraryID: titleID}}
	for _, f := range rows {
		if f.Kind != domain.FileVideo {
			continue
		}
		s, c := f.Place.Season, f.Place.First
		title := domain.ChapterTitleFromName(f.Rel, s, c, c, "")
		req.Videos = append(req.Videos, application.EditVideo{FileID: f.ID, Season: &s, Chapter: &c, ChapterTitle: title})
	}
	return req
}

// setChapter moves the chapter of the file at rel to chapter n.
func setChapter(req application.EditRequest, rows map[string]domain.LibraryFile, rel string, n int) application.EditRequest {
	for i := range req.Videos {
		if rows[rel].ID == req.Videos[i].FileID {
			c := n
			req.Videos[i].Chapter = &c
		}
	}
	return req
}

// TestAChapterThatClashesIsAskedAndTheReplaceDecisionTrashesTheOther checks RF-24 on a chapter: Replace sends the
// file that holds the chapter to the trash, with its subtitles, and the edited file takes its place.
func TestAChapterThatClashesIsAskedAndTheReplaceDecisionTrashesTheOther(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	titleID, rows := e.seedSeries()
	pilot := lizzieFolder + "/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv"
	trial := lizzieFolder + "/Season 01/Lizzie (2008) - S01E02 - Trial.mkv"
	req := setChapter(chapterChoices(rows, titleID), rows, trial, 1)

	p := e.plan(titleID, req)
	if p.Valid {
		t.Fatal("a chapter taken by another file needs a decision")
	}
	req = withRevision(req, p.Revision)
	for i := range req.Videos {
		if rows[trial].ID == req.Videos[i].FileID {
			req.Videos[i].Duplicate = domain.DecisionReplace
		}
	}
	p = e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("with Replace: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	if e.exists(pilot) || e.exists(trial) || !e.exists(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Trial.mkv") {
		t.Error("the chapter 1 is the trial now; the pilot is in the trash")
	}
	after := e.rows(titleID)
	var pilotTrashed bool
	for _, row := range after {
		if row.TrashRel == pilot && row.TrashEntryID != 0 && strings.HasPrefix(row.Rel, "trash:") {
			pilotTrashed = true
		}
	}
	if !pilotTrashed {
		t.Error("the replaced file is in the trash, in the registry too")
	}
}

// TestKeepingBothVersionsOfAChapterLabelsThem checks RF-24 on a chapter: the old file takes the label the user
// wrote for it, and the new one takes its own.
func TestKeepingBothVersionsOfAChapterLabelsThem(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	titleID, rows := e.seedSeries()
	pilot := lizzieFolder + "/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv"
	trial := lizzieFolder + "/Season 01/Lizzie (2008) - S01E02 - Trial.mkv"
	req := setChapter(chapterChoices(rows, titleID), rows, trial, 1)
	for i := range req.Videos {
		if rows[trial].ID == req.Videos[i].FileID {
			req.Videos[i].Duplicate = domain.DecisionKeepBoth
			req.Videos[i].Version = "720p"
		}
	}
	req.Labels = []application.LabelChoice{{Rel: pilot, Version: "1080p"}}
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("keep both: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	base := lizzieFolder + "/Season 01/Lizzie (2008) - S01E01 - Pilot - 1080p.mkv"
	if !e.exists(base) {
		t.Errorf("the old version takes its label: %s", base)
	}
	if !e.exists(lizzieFolder + "/Season 01/Lizzie (2008) - S01E01 - Trial - 720p.mkv") {
		t.Error("the edited chapter takes its own label")
	}
	if e.exists(pilot) {
		t.Error("the old name is gone")
	}
}

// TestSkippingADuplicateKeepsTheChapterWhereItIs checks RF-24 Omitir on a chapter: the edited file stays.
func TestSkippingADuplicateKeepsTheChapterWhereItIs(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	titleID, rows := e.seedSeries()
	trial := lizzieFolder + "/Season 01/Lizzie (2008) - S01E02 - Trial.mkv"
	req := setChapter(chapterChoices(rows, titleID), rows, trial, 1)
	for i := range req.Videos {
		if rows[trial].ID == req.Videos[i].FileID {
			req.Videos[i].Duplicate = domain.DecisionSkip
		}
	}
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("skip: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	if !e.exists(trial) {
		t.Error("the skipped chapter stays where it is")
	}
}

// TestTwoChaptersSwapTheirPlaces checks that a file never takes the place of a file that has not moved yet.
func TestTwoChaptersSwapTheirPlaces(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	titleID, rows := e.seedSeries()
	pilot := lizzieFolder + "/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv"
	trial := lizzieFolder + "/Season 01/Lizzie (2008) - S01E02 - Trial.mkv"
	req := setChapter(setChapter(chapterChoices(rows, titleID), rows, pilot, 2), rows, trial, 1)
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("swap: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	if !e.exists(lizzieFolder+"/Season 01/Lizzie (2008) - S01E02 - Pilot.mkv") || !e.exists(lizzieFolder+"/Season 01/Lizzie (2008) - S01E01 - Trial.mkv") {
		t.Error("the chapters swapped their names")
	}
}

// TestAChapterOverlappingAnotherIsAnIssue checks that two files cannot hold one chapter in one season.
func TestAChapterOverlappingAnotherIsAnIssue(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	titleID, rows := e.seedSeries()
	pilot := lizzieFolder + "/Season 01/Lizzie (2008) - S01E01 - Pilot.mkv"
	trial := lizzieFolder + "/Season 01/Lizzie (2008) - S01E02 - Trial.mkv"
	req := setChapter(chapterChoices(rows, titleID), rows, pilot, 2)
	req = setChapter(req, rows, trial, 2)
	if p := e.plan(titleID, req); p.Valid {
		t.Error("two files of one chapter are refused")
	}
}

// TestAFailureInTheMiddleUndoesTheWholeEdit makes one move fail and checks that nothing changed on disk or in the
// registry, and that the journal holds nothing (RNF-04: "si algo falla… se deshace todo").
func TestAFailureInTheMiddleUndoesTheWholeEdit(t *testing.T) {
	t.Parallel()
	failed := false
	e := newEditEnv(t, func(f application.Files) application.Files { return failingFiles{Files: f, failed: &failed} })
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	sub := e.write(ironFolder+"/Iron Man (2008).es.srt", "sub")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, video, domain.Place{}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("plan: %+v", p.Errors)
	}
	if _, err := e.edits.Commit(e.ctx, titleID, withRevision(req, p.Revision)); err == nil {
		t.Fatal("the commit fails at the injected step")
	}
	if !e.exists(video) || !e.exists(sub) || e.exists("Movies/Iron Man (2008) [tmdbid-1726]") {
		t.Error("everything is back where it was")
	}
	if title, ok := e.titleOf(ironFolder); !ok || title.TMDBID != 0 {
		t.Errorf("the registry title is unchanged: %+v, %v", title, ok)
	}
	if got := e.rows(titleID); got[video].ID == 0 || got[sub].Language != "es" {
		t.Errorf("the registry rows are unchanged: %+v", got)
	}
	e.assertNothingJournaled()
}

// failingFiles fails once the move of a subtitle to its final name, in the tagged folder: the video has moved by
// then, so the undo has work to do.
type failingFiles struct {
	application.Files
	failed *bool
}

// Move implements application.Files: it refuses one subtitle move, and moves the rest.
func (f failingFiles) Move(from, to string) error {
	if strings.HasSuffix(to, ".srt") && strings.Contains(to, "[tmdbid-1726]") && !*f.failed {
		*f.failed = true
		return errors.New("injected failure")
	}
	return f.Files.Move(from, to)
}

// TestSubtitleLanguageRenamesTheSubtitle changes the language of a subtitle and renames it (RF-23, RF-44).
func TestSubtitleLanguageRenamesTheSubtitle(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	sub := e.write(ironFolder+"/Iron Man (2008).es.srt", "sub")
	titleID := e.seedMovie(ironFolder, 0, false,
		e.video(domain.KindMovie, video, domain.Place{}, ""),
		mustSubtitle(t, domain.SubtitleSpec{Rel: sub, VideoRel: video, Language: "es", Size: 3}))
	rows := e.rows(titleID)
	req := movieChoices(rows, application.TitleChoice{Source: "library", LibraryID: titleID})
	for i := range req.Subtitles {
		req.Subtitles[i].Language = "en"
		req.Subtitles[i].Forced = true
	}
	p := e.plan(titleID, req)
	if !p.Valid {
		t.Fatalf("plan: %+v", p.Errors)
	}
	e.commit(titleID, withRevision(req, p.Revision))
	if e.exists(sub) || !e.exists(ironFolder+"/Iron Man (2008).en.forced.srt") || !e.exists(video) {
		t.Error("the subtitle takes its language and flag in its name; the video keeps its name")
	}
}

// TestMergingIntoAnotherTitleMovesItsFiles joins a free movie into a linked title of the library: the files
// move to its folder and the free title goes (RF-44: "Cambiar a otro título existente").
func TestMergingIntoAnotherTitleMovesItsFiles(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	linked := "Movies/Iron Man (2008) [tmdbid-1726]"
	old := e.write(linked+"/Iron Man (2008) [tmdbid-1726] - 1080p.mkv", "old")
	linkedID := e.seedMovie(linked, 1726, true, e.video(domain.KindMovie, old, domain.Place{}, "1080p"))
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "new")
	freeID := e.seedMovie(ironFolder, 0, false, e.video(domain.KindMovie, video, domain.Place{}, ""))
	rows := e.rows(freeID)
	req := movieChoices(rows, application.TitleChoice{Source: "library", LibraryID: linkedID})
	for i := range req.Videos {
		req.Videos[i].Duplicate = domain.DecisionKeepBoth
		req.Videos[i].Version = "2160p"
	}
	p := e.plan(freeID, req)
	if !p.Valid || p.MergeInto != linkedID {
		t.Fatalf("plan: valid=%v merge=%d errors=%+v", p.Valid, p.MergeInto, p.Errors)
	}
	res := e.commit(freeID, withRevision(req, p.Revision))
	if !res.Merged || res.TitleID != linkedID {
		t.Errorf("result = %+v", res)
	}
	if !e.exists(linked+"/Iron Man (2008) [tmdbid-1726] - 2160p.mkv") || !e.exists(linked+"/Iron Man (2008) [tmdbid-1726] - 1080p.mkv") {
		t.Error("both versions live in the linked title's folder")
	}
	if e.exists(ironFolder) {
		t.Error("the free folder is gone")
	}
	if _, ok := e.titleOf(ironFolder); ok {
		t.Error("the free title is gone from the registry")
	}
}

// TestASkipIsNotOfferedWhenTheFolderChanges checks that a file cannot stay behind when its title moves to another
// folder: the answer Omitir is refused for that case.
func TestASkipIsNotOfferedWhenTheFolderChanges(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write(ironFolder+"/Iron Man (2008).mkv", "iron")
	titleID := e.seedMovie(ironFolder, 0, false, e.video(domain.KindMovie, video, domain.Place{}, ""))
	// A file already holds the linked folder's place on the disk, so the new name is a duplicate.
	e.write("Movies/Iron Man (2008) [tmdbid-1726]/Iron Man (2008) [tmdbid-1726].mkv", "taken")
	req := movieChoices(e.rows(titleID), application.TitleChoice{Source: "tmdb", Kind: domain.KindMovie, TMDBID: 1726})
	for i := range req.Videos {
		req.Videos[i].Duplicate = domain.DecisionSkip
	}
	p := e.plan(titleID, req)
	if p.Valid {
		t.Error("Omitir is refused when the title's folder changes")
	}
}

// TestMoveOfAChapterThatTMDBDoesNotKnowIsAWarning keeps an edit valid when TMDB does not know the season (RF-22).
func TestAChapterOfASeasonTMDBDoesNotKnowIsAWarning(t *testing.T) {
	t.Parallel()
	e := newEditEnv(t, nil)
	video := e.write("TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv", "bb")
	titleID := recordTitle(t, e.conn, mustTitle(t, domain.KindSeries, "Breaking Bad", "TV Shows/Breaking Bad (2008)", 0, false),
		e.video(domain.KindSeries, video, domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""))
	rows := e.rows(titleID)
	req := application.EditRequest{Title: application.TitleChoice{Source: "tmdb", Kind: domain.KindSeries, TMDBID: 1396}}
	s, c := 9, 1
	for _, f := range rows {
		req.Videos = append(req.Videos, application.EditVideo{FileID: f.ID, Season: &s, Chapter: &c})
	}
	p := e.plan(titleID, req)
	if !p.Valid || len(p.Warnings) == 0 {
		t.Errorf("an unknown season is a warning: valid=%v warnings=%+v errors=%+v", p.Valid, p.Warnings, p.Errors)
	}
}
