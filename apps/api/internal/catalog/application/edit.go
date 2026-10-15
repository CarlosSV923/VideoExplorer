package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Editing a title (spec RF-44): the form sends the title's new identity and the place of each of its
// files, the server plans the renames and the duplicates (RF-24) and commits them as one change of the
// library. The file names are never trusted: every path comes from the registry and §5.

// ErrPlanChanged means the title changed between the preview and the commit: the preview is checked again
// (RF-25, decision 2026-10-14: the form has no lock).
var ErrPlanChanged = errors.New("the title changed since the plan was made")

// EditRequest is what the edit form sends for a title (RF-44). Title is the title the files get: a title of
// the library (merged into), a TMDB title or a free name. Every video and subtitle of the title needs a choice.
type EditRequest struct {
	// Revision is the revision of the title that the plan was made from (plan_changed, RF-25).
	Revision  string
	Title     TitleChoice
	Videos    []EditVideo
	Subtitles []EditSubtitle
	// Labels are the labels written for the old versions that need one (RF-24), as the plan names them.
	Labels []LabelChoice
}

// EditVideo is what one video of the title becomes: its place (a movie's part, or a series' chapters), its
// version label (RF-24), the chapter title of a free series, and the answer to a duplicate of its place.
type EditVideo struct {
	FileID       int64
	Part         int
	Season       *int
	Chapter      *int
	LastChapter  *int
	ChapterTitle string
	Version      string
	Duplicate    domain.Decision
}

// EditSubtitle is what one subtitle of the title becomes (RF-23): its language and its flags.
type EditSubtitle struct {
	FileID   int64
	Language string
	Forced   bool
	SDH      bool
}

// EditIssue is one thing that an edit gets wrong: the file it is about (0 for the title), the field, a stable
// code the UI translates, and the path of the version it is about when it is about one (RF-24).
type EditIssue struct {
	FileID int64
	File   string
	Field  string
	Code   string
	Target string
}

// EditPlan is the preview of an edit (RF-44): the title's identity after the edit, and where each file goes.
// Valid is true when the same request can be committed. Nothing is changed by a plan.
type EditPlan struct {
	TitleID  int64
	Revision string
	Valid    bool
	Kind     domain.TitleKind
	Title    string
	Folder   string
	// MergeInto is the title the files move to when the edit joins another title of the library (RF-44); 0 otherwise.
	MergeInto int64
	Videos    []EditedVideo
	Subtitles []EditedSubtitle
	Errors    []EditIssue
	Warnings  []EditIssue
}

// EditedVideo is a video of the title in the preview: its path before and after, and what its duplicates allow.
type EditedVideo struct {
	FileID    int64
	File      string
	FinalPath string
	Changed   bool
	// Duplicates are the videos that hold the place the video goes to (RF-24), and the answers they accept.
	Duplicates        []PlannedDuplicate
	ExactCopy         bool
	Allowed           []domain.Decision
	Proposal          domain.Decision
	Decision          domain.Decision
	Version           string
	VersionProposal   string
	VersionIncomplete bool
}

// EditedSubtitle is a subtitle of the title in the preview.
type EditedSubtitle struct {
	FileID    int64
	File      string
	FinalPath string
	Changed   bool
}

// EditResult says what an edit did: the title it ended in, its folder, whether it joined another title, and
// whether anything changed at all.
type EditResult struct {
	TitleID int64
	Folder  string
	Merged  bool
	Changed bool
}

// EditInvalidError is returned by a commit whose request is not valid: the plan says what is wrong.
type EditInvalidError struct {
	Plan EditPlan
}

// Error describes the refusal.
func (e *EditInvalidError) Error() string { return ErrPlanInvalid.Error() }

// Unwrap reports ErrPlanInvalid.
func (e *EditInvalidError) Unwrap() error { return ErrPlanInvalid }

// EditDeps are what the edit needs; the composition root wires them.
type EditDeps struct {
	Library LibraryRepository
	Files   Files
	Engine  *Engine
	Finder  *DuplicateFinder
	Media   MediaReader
	TMDB    TMDB
	Roots   Roots
	// SubtitleExts are the subtitle extensions with their dot (SUBTITLE_EXTENSIONS).
	SubtitleExts []string
	// Now and Log may be nil.
	Now func() time.Time
	Log *slog.Logger
}

// Edits changes the titles of the library: the plan and the commit of an edit (RF-44).
type Edits struct {
	d      EditDeps
	subExt map[string]bool
	now    func() time.Time
	log    *slog.Logger
}

// NewEdits builds the edit use cases from their ports.
func NewEdits(d EditDeps) *Edits {
	e := &Edits{d: d, subExt: extSet(d.SubtitleExts), now: d.Now, log: d.Log}
	if e.now == nil {
		e.now = time.Now
	}
	if e.log == nil {
		e.log = slog.New(slog.DiscardHandler)
	}
	return e
}

// editState is a title with its files as the registry holds them, read before a plan or a commit.
type editState struct {
	title    domain.LibraryTitle
	files    []domain.LibraryFile
	live     []domain.LibraryTitle
	trashed  []domain.LibraryTitle
	revision string
}

// load reads the title and its files. A title with no file out of the trash is not in the library (RF-30).
func (e *Edits) load(ctx context.Context, titleID int64) (editState, error) {
	live, trashed, err := classifyTitles(ctx, e.d.Library)
	if err != nil {
		return editState{}, err
	}
	var st editState
	found := false
	for _, t := range live {
		if t.ID == titleID {
			st.title, found = t, true
		}
	}
	if !found {
		return editState{}, ErrTitleNotFound
	}
	st.live, st.trashed = live, trashed
	st.files, err = e.d.Library.Files(ctx, titleID)
	if err != nil {
		return editState{}, err
	}
	st.revision = revisionOf(st.title, st.files)
	return st, nil
}

// revisionOf returns the revision of a title: a digest of its registry values, so that a plan can tell that
// the title changed under it (RF-25).
func revisionOf(t domain.LibraryTitle, files []domain.LibraryFile) string {
	lines := []string{fmt.Sprintf("title|%d|%s|%s|%s|%d|%d|%t|%s", t.ID, t.Kind, t.Title, t.DiscTitle, t.Year, t.TMDBID, t.IDTag, t.Folder)}
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("file|%d|%d|%s|%s|%t|%d|%d|%d|%d|%s|%s|%t|%t|%d",
			f.ID, f.TitleID, f.Kind, f.Rel, f.Place.Episode, f.Place.Season, f.Place.First, f.Place.Last, f.Place.Part,
			f.Version, f.Language, f.Forced, f.SDH, f.TrashEntryID))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// Plan checks an edit of a title and returns its preview (RF-44). It changes nothing.
func (e *Edits) Plan(ctx context.Context, titleID int64, req EditRequest) (EditPlan, error) {
	st, err := e.load(ctx, titleID)
	if err != nil {
		return EditPlan{}, err
	}
	ev, err := e.assess(ctx, st, req)
	if err != nil {
		return EditPlan{}, err
	}
	return ev.plan, nil
}

// editTitle is the title the files of an edit get (RF-44): the title itself, a title of the library the files
// move into, or a new name. Its folder is where the files go.
type editTitle struct {
	kind    domain.TitleKind
	title   domain.Title
	display string
	root    string
	folder  string
	// merge is the other title of the library the files move into; nil when the title keeps its own registry.
	merge *domain.LibraryTitle
	// tmdbID is the TMDB id of the title after the edit; 0 for a free title.
	tmdbID int64
}

// assess checks a request against the title as it is and decides every placement (RF-22 to RF-24, RF-44).
// It changes nothing: the commit runs the same assessment under the engine's lock.
func (e *Edits) assess(ctx context.Context, st editState, req EditRequest) (*editEval, error) {
	ev := newEditEval(st)
	ev.labels = map[string]string{}
	for _, l := range req.Labels {
		ev.labelRels = append(ev.labelRels, l.Rel)
		ev.labels[strings.ToLower(l.Rel)] = l.Version
	}
	id, err := e.resolveTitle(ctx, st, req.Title, ev)
	if err != nil {
		return nil, err
	}
	e.prepareFiles(st, req, ev)
	if id == nil {
		ev.finish(st, nil)
		return ev, nil
	}
	ev.id = id
	if err := e.placeFiles(ctx, ev); err != nil {
		return nil, err
	}
	if err := e.decideGroups(ctx, ev); err != nil {
		return nil, err
	}
	ev.checkFinals()
	ev.finish(st, id)
	return ev, nil
}

// resolveTitle returns the title the files get, or nil when the choice is not usable (the reason is an issue).
// A title of TMDB or a new name that the trash holds is not in the library (RF-30): its code is in_trash.
func (e *Edits) resolveTitle(ctx context.Context, st editState, c TitleChoice, ev *editEval) (*editTitle, error) {
	cur := st.title
	switch c.Source {
	case "library":
		for i := range st.live {
			lt := st.live[i]
			if lt.ID != c.LibraryID {
				continue
			}
			if lt.ID == cur.ID {
				return e.keep(cur), nil
			}
			return e.into(lt), nil
		}
		ev.addTitleIssue("missing")
		return nil, nil
	case "tmdb":
		if c.Kind != domain.KindMovie && c.Kind != domain.KindSeries {
			ev.addTitleIssue("kind_invalid")
			return nil, nil
		}
		if !e.d.TMDB.Configured() {
			ev.addTitleIssue("tmdb_not_configured")
			return nil, nil
		}
		if other := titleOfTMDB(st.live, c.Kind, c.TMDBID, cur.ID); other != nil {
			return e.into(*other), nil
		}
		if titleOfTMDB(st.trashed, c.Kind, c.TMDBID, cur.ID) != nil {
			ev.addTitleIssue("in_trash")
			return nil, nil
		}
		tt, err := e.d.TMDB.Title(ctx, c.Kind, c.TMDBID)
		switch {
		case isNotFound(err):
			ev.addTitleIssue("missing")
			return nil, nil
		case err != nil:
			ev.addTitleIssue("tmdb_unavailable")
			return nil, nil
		}
		idTag := cur.TMDBID == 0 || cur.IDTag
		t := domain.Title{Name: tt.DiscTitle, Year: tt.Year, TMDBID: c.TMDBID, IDTag: idTag}
		return e.named(st, c.Kind, t, tt.Title, ev)
	case "free":
		if c.Kind != domain.KindMovie && c.Kind != domain.KindSeries {
			ev.addTitleIssue("kind_invalid")
			return nil, nil
		}
		name := strings.TrimSpace(c.Title)
		if name == "" {
			ev.addTitleIssue("required")
			return nil, nil
		}
		if c.Year < 0 || c.Year > 9999 {
			ev.addIssue(0, "year", "invalid", "")
			return nil, nil
		}
		return e.named(st, c.Kind, domain.Title{Name: name, Year: c.Year}, name, ev)
	default:
		ev.addTitleIssue("source_invalid")
		return nil, nil
	}
}

// named returns the title of a new name (TMDB or free): its folder, and the other title that already holds it.
func (e *Edits) named(st editState, kind domain.TitleKind, t domain.Title, display string, ev *editEval) (*editTitle, error) {
	root := e.rootFor(kind)
	folder, err := domain.TitleFolder(root, t)
	if err != nil {
		ev.addTitleIssue("name_invalid")
		return nil, nil
	}
	out := &editTitle{kind: kind, title: t, display: display, root: root, folder: folder, tmdbID: t.TMDBID}
	for i := range st.live {
		lt := st.live[i]
		if lt.ID != st.title.ID && strings.EqualFold(lt.Folder, folder) {
			out.merge = &lt
			out.folder = lt.Folder
			out.kind, out.title, out.root, out.tmdbID = lt.Kind, domain.Title{Name: lt.DiscTitle, Year: lt.Year, TMDBID: lt.TMDBID, IDTag: lt.IDTag}, rootOf(lt.Folder), lt.TMDBID
			out.display = lt.Title
			return out, nil
		}
	}
	for i := range st.trashed {
		if strings.EqualFold(st.trashed[i].Folder, folder) {
			ev.addTitleIssue("in_trash")
			return nil, nil
		}
	}
	return out, nil
}

// keep is the title as it is: the identity of the title the edit started from.
func (e *Edits) keep(t domain.LibraryTitle) *editTitle {
	return &editTitle{
		kind: t.Kind, root: rootOf(t.Folder), folder: t.Folder, tmdbID: t.TMDBID, display: t.Title,
		title: domain.Title{Name: t.DiscTitle, Year: t.Year, TMDBID: t.TMDBID, IDTag: t.IDTag},
	}
}

// into is the title of the library another title's files move into (RF-44: "Cambiar a otro título existente").
func (e *Edits) into(lt domain.LibraryTitle) *editTitle {
	out := e.keep(lt)
	out.merge = &lt
	return out
}

// titleOfTMDB returns the title of kind with TMDB id tmdbID other than the one with id self, or nil.
func titleOfTMDB(titles []domain.LibraryTitle, kind domain.TitleKind, tmdbID, self int64) *domain.LibraryTitle {
	if tmdbID == 0 {
		return nil
	}
	for i := range titles {
		if titles[i].Kind == kind && titles[i].TMDBID == tmdbID && titles[i].ID != self {
			return &titles[i]
		}
	}
	return nil
}

// rootFor returns MOVIES_DIR or SERIES_DIR for a kind.
func (e *Edits) rootFor(kind domain.TitleKind) string {
	if kind == domain.KindSeries {
		return e.d.Roots.Series
	}
	return e.d.Roots.Movies
}

// classifyTitles splits the registry's titles into the ones in the library and the ones whose files are all in
// the trash (RF-30). A title with no files at all is in neither.
func classifyTitles(ctx context.Context, repo LibraryRepository) (live, trashed []domain.LibraryTitle, err error) {
	titles, err := repo.Titles(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, t := range titles {
		files, err := repo.Files(ctx, t.ID)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case anyLive(files):
			live = append(live, t)
		case len(files) > 0:
			trashed = append(trashed, t)
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return live[i].ID < live[j].ID })
	return live, trashed, nil
}

// extOf returns the lower-case extension of a file name, with its dot.
func extOf(rel string) string {
	return strings.ToLower(path.Ext(rel))
}

// editTarget is the place a video takes, for the duplicates search (RF-22, RF-24).
func editTarget(kind domain.TitleKind, place domain.Place) domain.Target {
	t := domain.Target{Kind: kind}
	if kind == domain.KindMovie {
		t.Parts = place.Part > 0
	} else {
		t.Place = place
	}
	return t
}

// isLive reports whether a registry row is out of the trash.
func isLive(f domain.LibraryFile) bool { return f.Live() }

// mediaOf reads what a version label needs from a video of the library (RF-24). A video that cannot be read has
// no media, so its label stays incomplete.
func (e *Edits) mediaOf(ctx context.Context, rel string) (domain.VersionMedia, error) {
	if e.d.Media == nil {
		return domain.VersionMedia{}, nil
	}
	return e.d.Media.Media(ctx, e.d.Files.LibraryPath(rel))
}

// errorsAsName reports whether err is a name the server cannot build (spec §5), and returns it.
func errorsAsName(err error) (*domain.NameError, bool) {
	var ne *domain.NameError
	if errors.As(err, &ne) {
		return ne, true
	}
	return nil, false
}
