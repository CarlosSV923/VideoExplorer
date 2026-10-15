package application

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Choices of the form (RF-21 to RF-24). Every video and subtitle of the entry must be answered;
// the file names are the paths the entry holds, and the server builds every library path itself.

// Request is what the form sends for an entry: what each video and each subtitle becomes.
type Request struct {
	Videos    []VideoChoice
	Subtitles []SubtitleChoice
	// Labels are the labels written for the old versions that need one (RF-24).
	Labels []LabelChoice
}

// LabelChoice is the label the user wrote for a version that already holds the place of a video and has
// no readable label of its own (RF-24). Rel is the path of that version inside the library, as the plan
// names it; Version is cleaned like the label of a new version.
type LabelChoice struct {
	Rel     string
	Version string
}

// TitleChoice is the title a video goes to (RF-21): a title of the registry, a TMDB title, or a free
// name typed by the user (spec §6). Kind is the kind of a TMDB or a free title.
type TitleChoice struct {
	Source    string
	LibraryID int64
	TMDBID    int64
	Kind      domain.TitleKind
	Title     string
	Year      int
}

// VideoChoice is what one video of the entry becomes (RF-22): kept in No asignados (Keep), or placed
// in a title as a part of a movie or as chapters of a series. Version is the label the user chose for
// Keep both (RF-24, editable); Duplicate answers the duplicate, when there is one.
type VideoChoice struct {
	File  string
	Keep  bool
	Title TitleChoice
	// Part is the number of a movie's part; 0 for the whole movie.
	Part int
	// Season, Chapter and LastChapter place a chapter (RF-22); Season 0 holds the specials.
	Season, Chapter, LastChapter *int
	// ChapterTitle is the chapter's title for a free series (spec §6); "" when it has none.
	ChapterTitle string
	Version      string
	Duplicate    domain.Decision
}

// SubtitleChoice is what one subtitle becomes (RF-23): the video it goes with ("" for none), its
// language and its flags.
type SubtitleChoice struct {
	File     string
	Video    string
	Language string
	Forced   bool
	SDH      bool
}

// Plan is the preview of a request (RF-25): where each file goes, the duplicates and what the user
// can answer to them, and what is wrong. Valid is true when the request can be committed. Nothing is
// changed by a plan.
type Plan struct {
	EntryID   int64
	Valid     bool
	Videos    []PlannedVideo
	Subtitles []PlannedSubtitle
	Errors    []FieldIssue
	Warnings  []FieldIssue
}

// FieldIssue is one thing that a choice gets wrong: the file it is about ("" for the entry), the field
// and a stable code the UI translates (RF-22, RF-24, RF-25).
type FieldIssue struct {
	File  string
	Field string
	Code  string
	// Target is the path inside the library of the version the issue is about, when it is about one (RF-24).
	Target string
}

// PlannedVideo is a video of the entry in the preview. FinalPath is its library path once placed
// (spec §5), "" when it stays in No asignados. Allowed lists the answers a duplicate accepts, and
// Proposal is the answer offered first (RF-24: an exact copy is skipped).
type PlannedVideo struct {
	File       string
	Keep       bool
	FinalPath  string
	Duplicates []PlannedDuplicate
	ExactCopy  bool
	Allowed    []domain.Decision
	Proposal   domain.Decision
	Decision   domain.Decision
	// Version is the label kept with Keep both; VersionProposal is the one the rule proposes, and
	// VersionIncomplete says it cannot name a part (RF-24).
	Version           string
	VersionProposal   string
	VersionIncomplete bool
}

// PlannedDuplicate is a video that already holds the place of a planned video (RF-24).
type PlannedDuplicate struct {
	Rel       string
	Size      int64
	Version   string
	ExactCopy bool
	// VersionProposal is the label the rule proposes for this version when both are kept. VersionNeeded
	// says that the rule cannot name it: the user writes its label in the request (RF-24).
	VersionProposal string
	VersionNeeded   bool
}

// PlannedSubtitle is a subtitle in the preview: the video it goes with and its final path, "" when
// it stays with its video in No asignados.
type PlannedSubtitle struct {
	File      string
	Video     string
	FinalPath string
}

// PlanInvalidError is returned by a commit whose request is not valid: the plan says what is wrong.
type PlanInvalidError struct {
	Plan Plan
}

// Error describes the refusal.
func (e *PlanInvalidError) Error() string { return ErrPlanInvalid.Error() }

// Unwrap reports ErrPlanInvalid.
func (e *PlanInvalidError) Unwrap() error { return ErrPlanInvalid }

// target is the title that a placed video goes to: an existing title of the registry, or a title that
// the commit creates (newTitle).
type target struct {
	kind      domain.TitleKind
	folder    string
	title     domain.Title
	root      string
	display   string
	libraryID int64
	tmdbID    int64
	newTitle  bool
}

// videoState is a video of the entry while a request is checked.
type videoState struct {
	rel     string
	size    int64
	chosen  bool
	choice  VideoChoice
	keep    bool
	issues  []FieldIssue
	warns   []FieldIssue
	target  *target
	place   domain.Place
	chapter string
	// Set by the assessment.
	group     *placement
	incoming  domain.Incoming
	dups      domain.Duplicates
	occupants []domain.Occupant
	hasDups   bool
	planned   bool
	finalPath string
	decision  domain.Decision
	version   string
	labelProp string
	labelBad  bool
	// occLabels are the labels the rule proposes for the versions of the place, then the new video's.
	occLabels []domain.VersionLabel
}

// subtitleState is a subtitle of the entry while a request is checked.
type subtitleState struct {
	rel       string
	size      int64
	choice    SubtitleChoice
	chosen    bool
	video     *videoState
	language  string
	issues    []FieldIssue
	finalPath string
	planned   bool
}

// placement is one video, or the parts of one movie, that take one place of a title (RF-22). It has
// its own decision on the duplicates of that place (RF-24).
type placement struct {
	key     string
	videos  []*videoState
	target  *target
	plan    domain.DecisionPlan
	perform bool
}

// prepared is a request with its titles and places read, before the disk is read.
type prepared struct {
	// labels are the labels the user wrote, by the key of their version; labelOrder keeps their order and
	// labelUsed marks the ones that address a version of a place.
	labels     map[string]string
	labelOrder []string
	labelUsed  map[string]bool
	entry      domain.UnassignedEntry
	videos     []*videoState
	subtitles  []*subtitleState
	issues     []FieldIssue
	warns      []FieldIssue
}

// Plan checks a request for the entry and returns its preview (RF-25). It renews the lock of the tab
// that asks (RF-26), and changes nothing else.
func (a *Assignments) Plan(ctx context.Context, entryID int64, token string, req Request) (Plan, error) {
	entry, err := a.entry(ctx, entryID)
	if err != nil {
		return Plan{}, err
	}
	if err := a.requireLock(entry, token); err != nil {
		return Plan{}, err
	}
	now := a.now()
	if ok, err := a.d.Unassigned.Lock(ctx, entryID, token, now, now.Add(domain.AssignLockTTL)); err != nil {
		return Plan{}, err
	} else if !ok {
		return Plan{}, ErrAssigning
	}
	p, err := a.prepare(ctx, entry, req)
	if err != nil {
		return Plan{}, err
	}
	ev, err := a.assess(ctx, p)
	if err != nil {
		return Plan{}, err
	}
	return ev.plan, nil
}

// prepare reads the titles and the places of a request: it checks which file each choice names, and
// reads TMDB for the titles and chapters that need it (spec §6). It reads the registry, not the disk.
func (a *Assignments) prepare(ctx context.Context, entry domain.UnassignedEntry, req Request) (*prepared, error) {
	p := &prepared{entry: entry, labels: map[string]string{}, labelUsed: map[string]bool{}}
	titles, trashed, err := classifyTitles(ctx, a.d.Library)
	if err != nil {
		return nil, err
	}
	known := map[string]domain.UnassignedFile{}
	for _, f := range entry.Files {
		known[f.Rel] = f
	}

	videoChoice := map[string]VideoChoice{}
	for _, c := range req.Videos {
		if _, ok := known[c.File]; !ok || !a.isVideo(entryPath(entry, c.File)) {
			p.issues = append(p.issues, FieldIssue{File: c.File, Field: "file", Code: "unknown"})
			continue
		}
		if _, dup := videoChoice[c.File]; dup {
			p.issues = append(p.issues, FieldIssue{File: c.File, Field: "file", Code: "duplicate"})
			continue
		}
		videoChoice[c.File] = c
	}
	subChoice := map[string]SubtitleChoice{}
	for _, c := range req.Subtitles {
		if _, ok := known[c.File]; !ok || !a.isSubtitle(entryPath(entry, c.File)) {
			p.issues = append(p.issues, FieldIssue{File: c.File, Field: "file", Code: "unknown"})
			continue
		}
		if _, dup := subChoice[c.File]; dup {
			p.issues = append(p.issues, FieldIssue{File: c.File, Field: "file", Code: "duplicate"})
			continue
		}
		subChoice[c.File] = c
	}

	for _, l := range req.Labels {
		key := strings.ToLower(l.Rel)
		if _, dup := p.labels[key]; dup {
			p.issues = append(p.issues, FieldIssue{Field: "version", Code: "duplicate", Target: l.Rel})
			continue
		}
		p.labels[key] = l.Version
		p.labelOrder = append(p.labelOrder, l.Rel)
	}
	for _, f := range entry.Files {
		switch {
		case a.isVideo(entryPath(entry, f.Rel)):
			vs := &videoState{rel: f.Rel, size: f.Size}
			p.videos = append(p.videos, vs)
			c, ok := videoChoice[f.Rel]
			if !ok {
				p.issues = append(p.issues, FieldIssue{File: f.Rel, Field: "video", Code: "missing"})
				continue
			}
			vs.chosen, vs.choice = true, c
			if c.Keep {
				vs.keep = true
				continue
			}
			if err := a.resolveVideo(ctx, vs, titles, trashed); err != nil {
				return nil, err
			}
		case a.isSubtitle(entryPath(entry, f.Rel)):
			ss := &subtitleState{rel: f.Rel, size: f.Size}
			p.subtitles = append(p.subtitles, ss)
			c, ok := subChoice[f.Rel]
			if !ok {
				p.issues = append(p.issues, FieldIssue{File: f.Rel, Field: "subtitle", Code: "missing"})
				continue
			}
			ss.chosen, ss.choice = true, c
		}
	}
	a.linkSubtitles(p)
	return p, nil
}

// resolveVideo reads the title and the place of a video that is placed, and its chapter title (RF-22,
// spec §5, §6). Problems become issues of the video.
func (a *Assignments) resolveVideo(ctx context.Context, vs *videoState, titles, trashed []domain.LibraryTitle) error {
	c := vs.choice
	t, field, code, err := a.resolveTitle(ctx, c.Title, titles, trashed)
	if err != nil {
		return err
	}
	if code != "" {
		vs.issues = append(vs.issues, FieldIssue{File: vs.rel, Field: field, Code: code})
		return nil
	}
	place, issue := domain.PlaceOf(t.kind, c.Part, c.Season, c.Chapter, c.LastChapter)
	if issue != (domain.Issue{}) {
		vs.issues = append(vs.issues, FieldIssue{File: vs.rel, Field: issue.Field, Code: issue.Code})
		return nil
	}
	vs.target, vs.place = t, place
	if t.kind != domain.KindSeries {
		return nil
	}
	if t.tmdbID == 0 {
		vs.chapter = strings.TrimSpace(c.ChapterTitle)
		return nil
	}
	chapters, err := a.d.TMDB.Chapters(ctx, t.tmdbID, place.Season, place.First, place.Last)
	switch {
	case isNotFound(err):
		vs.warns = append(vs.warns, FieldIssue{File: vs.rel, Field: "season", Code: "unknown"})
		return nil
	case err != nil:
		vs.issues = append(vs.issues, FieldIssue{File: vs.rel, Field: "chapter", Code: "tmdb_unavailable"})
		return nil
	}
	vs.chapter = chapters.Title
	if len(chapters.Missing) > 0 {
		vs.warns = append(vs.warns, FieldIssue{File: vs.rel, Field: "chapter", Code: "unknown"})
	}
	return nil
}

// resolveTitle returns the title that a choice names (RF-21). A problem is returned as a field and
// a code; an error is a failure of the registry.
func (a *Assignments) resolveTitle(ctx context.Context, c TitleChoice, titles, trashed []domain.LibraryTitle) (*target, string, string, error) {
	switch c.Source {
	case "library":
		for _, lt := range titles {
			if lt.ID != c.LibraryID {
				continue
			}
			return &target{
				kind: lt.Kind, folder: lt.Folder, root: rootOf(lt.Folder), libraryID: lt.ID, tmdbID: lt.TMDBID,
				title:   domain.Title{Name: lt.DiscTitle, Year: lt.Year, TMDBID: lt.TMDBID, IDTag: lt.IDTag},
				display: lt.Title,
			}, "", "", nil
		}
		return nil, "title", "missing", nil
	case "tmdb":
		if c.Kind != domain.KindMovie && c.Kind != domain.KindSeries {
			return nil, "title", "kind_invalid", nil
		}
		if !a.d.TMDB.Configured() {
			return nil, "title", "tmdb_not_configured", nil
		}
		if libraryIDOf(titles, c.Kind, c.TMDBID) > 0 {
			return nil, "title", "in_library", nil
		}
		if titleOfTMDB(trashed, c.Kind, c.TMDBID, 0) != nil {
			return nil, "title", "in_trash", nil
		}
		tt, err := a.d.TMDB.Title(ctx, c.Kind, c.TMDBID)
		switch {
		case isNotFound(err):
			return nil, "title", "missing", nil
		case err != nil:
			return nil, "title", "tmdb_unavailable", nil
		}
		title := domain.Title{Name: tt.DiscTitle, Year: tt.Year, TMDBID: c.TMDBID, IDTag: domain.DefaultIDTag(c.TMDBID)}
		return a.newTarget(c.Kind, title, tt.Title, titles, trashed)
	case "free":
		if c.Kind != domain.KindMovie && c.Kind != domain.KindSeries {
			return nil, "title", "kind_invalid", nil
		}
		name := strings.TrimSpace(c.Title)
		if name == "" {
			return nil, "title", "required", nil
		}
		if c.Year < 0 || c.Year > 9999 {
			return nil, "year", "invalid", nil
		}
		return a.newTarget(c.Kind, domain.Title{Name: name, Year: c.Year}, name, titles, trashed)
	default:
		return nil, "title", "source_invalid", nil
	}
}

// newTarget returns the target of a title the registry does not have yet: its folder must be free
// in the registry (RF-21: a title already there is assigned, not created again).
func (a *Assignments) newTarget(kind domain.TitleKind, title domain.Title, display string, titles, trashed []domain.LibraryTitle) (*target, string, string, error) {
	root := a.rootFor(kind)
	folder, err := domain.TitleFolder(root, title)
	if err != nil {
		return nil, "title", "name_invalid", nil
	}
	for _, lt := range titles {
		if strings.EqualFold(lt.Folder, folder) {
			return nil, "title", "in_library", nil
		}
	}
	for _, lt := range trashed {
		if strings.EqualFold(lt.Folder, folder) {
			return nil, "title", "in_trash", nil
		}
	}
	return &target{kind: kind, folder: folder, root: root, title: title, display: display, tmdbID: title.TMDBID, newTitle: true}, "", "", nil
}

// rootOf returns the library folder of a title folder ("Movies" for "Movies/Dune (2021)").
func rootOf(folder string) string {
	root, _, _ := strings.Cut(folder, "/")
	return root
}

// rootFor returns MOVIES_DIR or SERIES_DIR for a kind.
func (a *Assignments) rootFor(kind domain.TitleKind) string {
	if kind == domain.KindSeries {
		return a.d.Roots.Series
	}
	return a.d.Roots.Movies
}

// linkSubtitles checks the video each subtitle names: a video of the entry, and a language (RF-23).
func (a *Assignments) linkSubtitles(p *prepared) {
	videos := map[string]*videoState{}
	for _, vs := range p.videos {
		videos[vs.rel] = vs
	}
	for _, ss := range p.subtitles {
		if !ss.chosen {
			continue
		}
		c := ss.choice
		lang, err := domain.NormalizeLanguage(c.Language)
		if err != nil {
			ss.issues = append(ss.issues, FieldIssue{File: ss.rel, Field: "language", Code: "invalid"})
		} else {
			ss.language = lang
		}
		if c.Video != "" {
			vs, ok := videos[c.Video]
			if !ok {
				ss.issues = append(ss.issues, FieldIssue{File: ss.rel, Field: "video", Code: "unknown"})
			} else {
				ss.video = vs
			}
		}
		p.issues = append(p.issues, ss.issues...)
	}
}

// assess checks the prepared request against the disk and the registry, and decides each placement.
// It changes nothing. The plan it returns is what a commit performs when the plan is valid.
func (a *Assignments) assess(ctx context.Context, p *prepared) (*evaluation, error) {
	ev := &evaluation{}
	ev.issues = append(ev.issues, p.issues...)
	ev.warns = append(ev.warns, p.warns...)
	for _, vs := range p.videos {
		ev.issues = append(ev.issues, vs.issues...)
		ev.warns = append(ev.warns, vs.warns...)
	}

	layout := []domain.LayoutItem{}
	for _, vs := range p.videos {
		if placeable(vs) {
			layout = append(layout, domain.LayoutItem{File: vs.rel, Folder: vs.target.folder, Kind: vs.target.kind, Place: vs.place})
		}
	}
	for file, code := range domain.CheckLayout(layout) {
		for _, vs := range p.videos {
			if vs.rel == file {
				vs.issues = append(vs.issues, FieldIssue{File: file, Field: "place", Code: code})
				ev.issues = append(ev.issues, FieldIssue{File: file, Field: "place", Code: code})
			}
		}
	}

	groups := []*placement{}
	byKey := map[string]*placement{}
	for _, vs := range p.videos {
		if !placeable(vs) {
			continue
		}
		key := placementKey(vs)
		g := byKey[key]
		if g == nil {
			g = &placement{key: key, target: vs.target}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.videos = append(g.videos, vs)
		vs.group = g
	}
	for _, g := range groups {
		if err := a.assessPlacement(ctx, p, g, ev); err != nil {
			return nil, err
		}
	}

	for _, rel := range p.labelOrder {
		if !p.labelUsed[strings.ToLower(rel)] {
			ev.issues = append(ev.issues, FieldIssue{Field: "version", Code: "unknown", Target: rel})
		}
	}
	return ev.finish(p), nil
}

// placeable reports whether a video takes a place: it is chosen, not kept and checked without issues.
func placeable(vs *videoState) bool {
	return vs.chosen && !vs.keep && len(vs.issues) == 0 && vs.target != nil
}

// placementKey names the place a video takes: a movie's parts share one, a whole movie or a chapter
// stands alone.
func placementKey(vs *videoState) string {
	folder := strings.ToLower(vs.target.folder)
	switch {
	case vs.target.kind == domain.KindMovie && vs.place.Part > 0:
		return "parts|" + folder
	case vs.target.kind == domain.KindMovie:
		return "whole|" + folder + "|" + vs.rel
	default:
		return fmt.Sprintf("episode|%s|%d|%d|%d|%s", folder, vs.place.Season, vs.place.First, vs.place.Last, vs.rel)
	}
}

// assessPlacement decides the placement of one place: it reads the duplicates of its videos and plans
// the decision that the user gave to them, or the plain placement when there are none (RF-24).
func (a *Assignments) assessPlacement(ctx context.Context, p *prepared, g *placement, ev *evaluation) error {
	first := g.videos[0]
	decision := first.choice.Duplicate
	for _, vs := range g.videos[1:] {
		if vs.choice.Duplicate != decision {
			for _, v := range g.videos {
				v.addIssue(ev, "duplicate", "mismatch", "")
			}
			return nil
		}
	}
	subs := a.subtitlesOf(p, g)
	for _, vs := range g.videos {
		vs.incoming = a.incoming(p.entry, vs, subs[vs.rel])
		dups, err := a.d.Finder.Find(ctx, g.target.folder, a.targetOf(vs), vs.incoming.Source)
		if err != nil {
			return err
		}
		vs.dups = dups
		vs.hasDups = len(dups.Files) > 0
	}
	if !first.hasDups {
		incomings := make([]domain.Incoming, 0, len(g.videos))
		for _, vs := range g.videos {
			incomings = append(incomings, vs.incoming)
		}
		plan, err := domain.PlanPlacement(incomings)
		if err != nil {
			return err
		}
		g.plan, g.perform = plan, true
		for _, vs := range g.videos {
			vs.planned = true
		}
		ev.places = append(ev.places, g)
		return nil
	}

	occupants, err := a.d.Finder.Occupants(ctx, g.target.folder, first.dups, a.d.SubtitleExts, a.d.Media)
	if err != nil {
		return err
	}
	for _, vs := range g.videos {
		vs.occupants = occupants
		vs.incoming.Media, err = a.mediaOf(ctx, vs.incoming.Source)
		if err != nil {
			return err
		}
	}
	single := len(g.videos) == 1
	occ := append([]domain.Occupant(nil), occupants...)
	if single {
		labels := domain.ProposeVersions(occ, first.incoming)
		first.occLabels = labels
		first.labelProp, first.labelBad = labels[len(occ)].Text, labels[len(occ)].Incomplete
	}
	if decision == "" {
		p.markLabels(occ)
		for _, vs := range g.videos {
			vs.addIssue(ev, "duplicate", "required", "")
		}
		return nil
	}
	if decision != domain.DecisionKeepBoth || !single || first.dups.Exclusive {
		// Labels of the old versions are read only when both versions are kept, by one video of a place
		// that takes versions (RF-22, RF-24).
		p.markLabels(occ)
	} else if !applyLabels(p, first, occ, ev) {
		return nil
	}

	incomings := make([]domain.Incoming, 0, len(g.videos))
	for _, vs := range g.videos {
		in := vs.incoming
		if decision == domain.DecisionKeepBoth {
			in.Version = vs.choice.Version
		}
		incomings = append(incomings, in)
	}
	plan, err := domain.PlanDecision(decision, occ, first.dups.Exclusive, incomings)
	if err != nil {
		field, code, ok := decisionIssue(err)
		if !ok {
			return err
		}
		for _, vs := range g.videos {
			vs.addIssue(ev, field, code, "")
		}
		return nil
	}
	g.plan, g.perform = plan, decision != domain.DecisionSkip
	for _, vs := range g.videos {
		vs.decision = decision
		vs.planned = decision != domain.DecisionSkip
		vs.version = ""
		if decision == domain.DecisionKeepBoth {
			vs.version = plan.Version
		}
	}
	ev.places = append(ev.places, g)
	return nil
}

// applyLabels gives each old version that needs one the label the user wrote for it (RF-24). An old
// version with a label of its own, or with a complete proposal, needs none: a label written for it is an
// issue, and so is a version that needs one and has none. It reports whether every label is usable.
func applyLabels(p *prepared, vs *videoState, occ []domain.Occupant, ev *evaluation) bool {
	ok := true
	for i := range occ {
		rel := occ[i].Video.Rel
		key := strings.ToLower(rel)
		given, has := p.labels[key]
		if has {
			p.labelUsed[key] = true
		}
		needs := occ[i].Video.Version == "" && vs.occLabels[i].Incomplete
		switch {
		case has && !needs:
			vs.addIssue(ev, "version", "not_needed", rel)
			ok = false
		case has:
			if _, err := domain.CleanVersion(given); err != nil {
				vs.addIssue(ev, "version", "invalid", rel)
				ok = false
				continue
			}
			occ[i].Label = given
		case needs:
			vs.addIssue(ev, "version", "incomplete", rel)
			ok = false
		}
	}
	return ok
}

// markLabels marks the labels that address these versions as read, without applying them.
func (p *prepared) markLabels(occ []domain.Occupant) {
	for _, o := range occ {
		key := strings.ToLower(o.Video.Rel)
		if _, ok := p.labels[key]; ok {
			p.labelUsed[key] = true
		}
	}
}

// addIssue records an issue of the video, in the video and in the evaluation that reports it.
func (vs *videoState) addIssue(ev *evaluation, field, code, target string) {
	issue := FieldIssue{File: vs.rel, Field: field, Code: code, Target: target}
	vs.issues = append(vs.issues, issue)
	ev.issues = append(ev.issues, issue)
}

// decisionIssue names the field and the code of a decision that cannot be planned (RF-24).
func decisionIssue(err error) (field, code string, ok bool) {
	var nameErr *domain.NameError
	switch {
	case errors.Is(err, domain.ErrExclusive):
		return "duplicate", "not_allowed", true
	case errors.Is(err, domain.ErrVersionUnknown):
		return "version", "unknown", true
	case errors.Is(err, domain.ErrIncompleteLabel):
		return "version", "incomplete", true
	case errors.Is(err, domain.ErrInvalidVersion):
		return "version", "invalid", true
	case errors.As(err, &nameErr):
		return "version", "too_long", true
	default:
		return "", "", false
	}
}

// targetOf is the place a video takes, for the duplicates search (RF-22, RF-24).
func (a *Assignments) targetOf(vs *videoState) domain.Target {
	t := domain.Target{Kind: vs.target.kind}
	if vs.target.kind == domain.KindMovie {
		t.Parts = vs.place.Part > 0
	} else {
		t.Place = vs.place
	}
	return t
}

// incoming is a video of the entry as the placement needs it: where it waits and its name parts.
func (a *Assignments) incoming(entry domain.UnassignedEntry, vs *videoState, subs []domain.IncomingSubtitle) domain.Incoming {
	return domain.Incoming{
		Source:       libraryPath(entry, vs.rel),
		Kind:         vs.target.kind,
		Root:         vs.target.root,
		Title:        vs.target.title,
		Place:        vs.place,
		ChapterTitle: vs.chapter,
		Extension:    strings.ToLower(path.Ext(entryPath(entry, vs.rel))),
		Subtitles:    subs,
	}
}

// subtitlesOf returns, for each video of the placement, the subtitles that go with it (RF-23).
func (a *Assignments) subtitlesOf(p *prepared, g *placement) map[string][]domain.IncomingSubtitle {
	out := map[string][]domain.IncomingSubtitle{}
	for _, ss := range p.subtitles {
		if ss.video == nil || !ss.chosen || len(ss.issues) > 0 || ss.video.group != g {
			continue
		}
		out[ss.video.rel] = append(out[ss.video.rel], domain.IncomingSubtitle{
			Source:    libraryPath(p.entry, ss.rel),
			Language:  ss.language,
			Forced:    ss.choice.Forced,
			SDH:       ss.choice.SDH,
			Extension: strings.ToLower(path.Ext(entryPath(p.entry, ss.rel))),
		})
	}
	return out
}

// mediaOf reads what a video's label needs (RF-24). A video the reading fails for has no media, and
// its label stays incomplete (the adapter answers so).
func (a *Assignments) mediaOf(ctx context.Context, libraryRel string) (domain.VersionMedia, error) {
	if a.d.Media == nil {
		return domain.VersionMedia{}, nil
	}
	return a.d.Media.Media(ctx, a.d.Files.LibraryPath(libraryRel))
}

// allowedFor lists the answers a duplicate accepts: parts and versions are exclusive (RF-22).
func allowedFor(exclusive bool) []domain.Decision {
	if exclusive {
		return []domain.Decision{domain.DecisionReplace, domain.DecisionSkip}
	}
	return []domain.Decision{domain.DecisionReplace, domain.DecisionKeepBoth, domain.DecisionSkip}
}

// evaluation is what a check of a request decided: the plan, and the placements to perform.
type evaluation struct {
	plan   Plan
	issues []FieldIssue
	warns  []FieldIssue
	places []*placement
}

// finish builds the plan from the decisions: the planned videos and subtitles with their final paths,
// the duplicates, and the errors and warnings in the order of the entry.
func (ev *evaluation) finish(p *prepared) *evaluation {
	finals := map[string]string{}
	for _, g := range ev.places {
		for _, m := range g.plan.Place {
			finals[m.From] = m.To
		}
	}
	for _, vs := range p.videos {
		if vs.planned {
			vs.finalPath = finals[vs.incoming.Source]
		}
	}
	for _, ss := range p.subtitles {
		if ss.video != nil && ss.video.planned && len(ss.issues) == 0 && ss.chosen {
			ss.planned = true
			ss.finalPath = finals[libraryPath(p.entry, ss.rel)]
		}
	}
	plan := Plan{EntryID: p.entry.ID}
	for _, vs := range p.videos {
		plan.Videos = append(plan.Videos, plannedVideo(vs))
	}
	for _, ss := range p.subtitles {
		video := ""
		if ss.video != nil {
			video = ss.video.rel
		}
		plan.Subtitles = append(plan.Subtitles, PlannedSubtitle{File: ss.rel, Video: video, FinalPath: ss.finalPath})
	}
	plan.Errors = ev.issues
	plan.Warnings = ev.warns
	plan.Valid = len(plan.Errors) == 0
	ev.plan = plan
	return ev
}

// plannedVideo is a video in the preview.
func plannedVideo(vs *videoState) PlannedVideo {
	pv := PlannedVideo{File: vs.rel, Keep: vs.keep, FinalPath: vs.finalPath, Decision: vs.decision, Version: vs.version}
	if !vs.chosen || vs.keep || vs.target == nil {
		return pv
	}
	if vs.hasDups {
		pv.ExactCopy = vs.dups.ExactCopy
		pv.Allowed = allowedFor(vs.dups.Exclusive || unknownVersion(vs.occupants))
		if vs.dups.ExactCopy {
			pv.Proposal = domain.DecisionSkip
		}
	}
	for i, o := range vs.occupants {
		d := PlannedDuplicate{Rel: o.Video.Rel, Size: o.Video.Size, Version: o.Video.Version}
		if i < len(vs.occLabels) {
			d.VersionProposal = vs.occLabels[i].Text
			d.VersionNeeded = o.Video.Version == "" && vs.occLabels[i].Incomplete
		}
		pv.Duplicates = append(pv.Duplicates, d)
	}
	if vs.hasDups {
		pv.VersionProposal = vs.labelProp
		pv.VersionIncomplete = vs.labelBad
	}
	return pv
}

// unknownVersion reports whether a file of the place has no readable version (RF-24): then keeping both
// waits for its import (RF-29), and the preview does not offer it.
func unknownVersion(occupants []domain.Occupant) bool {
	for _, o := range occupants {
		if !o.Video.VersionKnown {
			return true
		}
	}
	return false
}
