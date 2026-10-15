package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// editEval is what an assessment of an edit decided: the title it ends in, every file with its choice and its
// place, the duplicates and the decisions of its placements, and the preview built from them.
type editEval struct {
	st        editState
	id        *editTitle
	videos    []*editVideo
	subs      []*editSub
	groups    []*editGroup
	videoByID map[int64]*editVideo
	subByID   map[int64]*editSub
	// labelUsed marks the labels the request gives to a version that a placement addresses (RF-24); labelRels
	// are the labels the request gives, in order, so that a label no placement uses is refused.
	labelUsed map[string]bool
	labelRels []string
	labels    map[string]string
	issues    []EditIssue
	warns     []EditIssue
	// trash are the replaced files of the duplicates (each video, then its subtitles); renames are the old
	// versions that take their label (keep both).
	trash   []domain.Move
	renames []domain.Move
	// moves are the moves of the title's own files, from where they are to where they go (From != To).
	moves []domain.Move
	plan  EditPlan
}

// editVideo is a video of the title while an edit is checked.
type editVideo struct {
	file   domain.LibraryFile
	subs   []*editSub
	chosen bool
	choice EditVideo
	place  domain.Place
	// version is the label the video takes: the one the form gives, or the one a decision computes (RF-24).
	version string
	chapter string
	// bad is true when the video has an issue of its own: its placement is not planned.
	bad bool
	// in is the video as the placement needs it (its title, place, subtitles).
	in domain.Incoming
	// moves are the moves of the video and its subtitles: the video first. Empty when it stays where it is.
	moves []domain.Move
	group *editGroup
	// skipped is true when the duplicates' answer Omitir keeps it where it is (RF-24).
	skipped bool
	// The duplicates of its place, when it has any, and the decision the form gave them.
	decision  domain.Decision
	occ       []domain.Occupant
	dups      domain.Duplicates
	hasDups   bool
	exclusive bool
	// occLabels are the labels the rule proposes for the versions of the place, then the video's own.
	occLabels []domain.VersionLabel
	// labelProp and labelBad are the label the rule proposes for the new video, and whether it is incomplete.
	labelProp string
	labelBad  bool
}

// editSub is a subtitle of the title while an edit is checked: it goes with its video (RF-23).
type editSub struct {
	file     domain.LibraryFile
	chosen   bool
	choice   EditSubtitle
	language string
	bad      bool
	video    *editVideo
}

// editGroup is the places of one destination: the parts of a movie share one, a whole movie or a chapter
// stands alone. A group with a file that moves has duplicates to check (RF-24).
type editGroup struct {
	key     string
	members []*editVideo
}

// newEditEval starts the assessment of a title.
func newEditEval(st editState) *editEval {
	return &editEval{st: st, videoByID: map[int64]*editVideo{}, subByID: map[int64]*editSub{}, labelUsed: map[string]bool{}}
}

// addIssue records an issue of the file fileID (0 for the title), and marks the video or subtitle it is about.
func (ev *editEval) addIssue(fileID int64, field, code, target string) {
	file := ""
	if v, ok := ev.videoByID[fileID]; ok {
		v.bad = true
		file = v.file.Rel
	}
	if s, ok := ev.subByID[fileID]; ok {
		s.bad = true
		file = s.file.Rel
	}
	ev.issues = append(ev.issues, EditIssue{FileID: fileID, File: file, Field: field, Code: code, Target: target})
}

// addTitleIssue records an issue of the title as a whole.
func (ev *editEval) addTitleIssue(code string) {
	ev.addIssue(0, "title", code, "")
}

// addMembers records the same issue for every video of a group.
func (ev *editEval) addMembers(members []*editVideo, field, code, target string) {
	for _, m := range members {
		ev.addIssue(m.file.ID, field, code, target)
	}
}

// prepareFiles matches the form's choices with the live files of the title: every video and subtitle needs one,
// and a choice that names no file of the title is refused (RF-44).
func (e *Edits) prepareFiles(st editState, req EditRequest, ev *editEval) {
	choiceV := map[int64]EditVideo{}
	for _, c := range req.Videos {
		if _, dup := choiceV[c.FileID]; dup {
			ev.addIssue(c.FileID, "file", "duplicate", "")
		}
		choiceV[c.FileID] = c
	}
	choiceS := map[int64]EditSubtitle{}
	for _, c := range req.Subtitles {
		if _, dup := choiceS[c.FileID]; dup {
			ev.addIssue(c.FileID, "file", "duplicate", "")
		}
		choiceS[c.FileID] = c
	}
	for _, f := range st.files {
		if !isLive(f) {
			continue
		}
		switch f.Kind {
		case domain.FileVideo:
			v := &editVideo{file: f}
			ev.videos = append(ev.videos, v)
			ev.videoByID[f.ID] = v
		case domain.FileSubtitle:
			s := &editSub{file: f}
			ev.subs = append(ev.subs, s)
			ev.subByID[f.ID] = s
		}
	}
	for id := range choiceV {
		if ev.videoByID[id] == nil {
			ev.addIssue(id, "file", "unknown", "")
		}
	}
	for id := range choiceS {
		if ev.subByID[id] == nil {
			ev.addIssue(id, "file", "unknown", "")
		}
	}
	for _, v := range ev.videos {
		c, ok := choiceV[v.file.ID]
		if !ok {
			ev.addIssue(v.file.ID, "video", "missing", "")
			continue
		}
		v.chosen, v.choice = true, c
	}
	for _, s := range ev.subs {
		c, ok := choiceS[s.file.ID]
		if !ok {
			ev.addIssue(s.file.ID, "subtitle", "missing", "")
			continue
		}
		s.chosen, s.choice = true, c
	}
	for _, s := range ev.subs {
		for _, v := range ev.videos {
			if strings.EqualFold(v.file.Rel, s.file.VideoRel) {
				s.video = v
				v.subs = append(v.subs, s)
			}
		}
	}
	// A subtitle whose video is not in the title cannot be placed.
	for _, s := range ev.subs {
		if s.video == nil {
			ev.addIssue(s.file.ID, "video", "unknown", "")
		}
	}
}

// finalPath returns where the file goes: the target of its planned move, or where it is.
func finalPath(moves []domain.Move, rel string) string {
	for _, m := range moves {
		if strings.EqualFold(m.From, rel) {
			return m.To
		}
	}
	return rel
}

// finalOfVideo returns where the video goes after the decisions.
func (v *editVideo) finalOfVideo() string {
	return finalPath(v.moves, v.file.Rel)
}

// movesOfNames returns the moves that belong to the files named by names (lower-case paths), in their order.
func movesOfNames(moves []domain.Move, names map[string]bool) []domain.Move {
	var out []domain.Move
	for _, m := range moves {
		if names[strings.ToLower(m.From)] {
			out = append(out, m)
		}
	}
	return out
}

// placeFiles decides the place of every chosen video and the language of every subtitle, and plans where
// the videos that place without a duplicate go (RF-22, RF-23, RF-44). Issues mark the file they are about.
func (e *Edits) placeFiles(ctx context.Context, ev *editEval) error {
	id := ev.id
	for _, v := range ev.videos {
		if !v.chosen {
			continue
		}
		c := v.choice
		place, issue := domain.PlaceOf(id.kind, c.Part, c.Season, c.Chapter, c.LastChapter)
		if issue != (domain.Issue{}) {
			ev.addIssue(v.file.ID, issue.Field, issue.Code, "")
			continue
		}
		v.place = place
		if id.kind == domain.KindMovie && place.Part > 0 && c.Version != "" {
			ev.addIssue(v.file.ID, "version", "not_allowed", "")
			continue
		}
		if c.Version != "" {
			clean, err := domain.CleanVersion(c.Version)
			if err != nil {
				ev.addIssue(v.file.ID, "version", "invalid", "")
				continue
			}
			v.version = clean
		}
		if err := e.chapterOf(ctx, id, v, ev); err != nil {
			return err
		}
	}
	for _, s := range ev.subs {
		if !s.chosen {
			continue
		}
		lang, err := domain.NormalizeLanguage(s.choice.Language)
		if err != nil {
			ev.addIssue(s.file.ID, "language", "invalid", "")
			continue
		}
		s.language = lang
	}
	for _, v := range ev.videos {
		if v.bad || !v.chosen {
			continue
		}
		v.in = e.incomingOf(id, v)
		moves, err := domain.PlacementMoves(v.in, v.version)
		if err != nil {
			ne, ok := errorsAsName(err)
			if !ok {
				return err
			}
			ev.addIssue(v.file.ID, ne.Field, "invalid", "")
			continue
		}
		v.moves = moves
	}
	ev.planLayout()
	ev.groupPlaces()
	return nil
}

// chapterOf reads the chapter title of a series video: the title of TMDB for a linked title (spec §5, §6), or
// the one the form gives for a free series (RF-44). A season TMDB does not know is a warning (RF-22).
func (e *Edits) chapterOf(ctx context.Context, id *editTitle, v *editVideo, ev *editEval) error {
	if id.kind != domain.KindSeries {
		return nil
	}
	if id.tmdbID == 0 {
		v.chapter = strings.TrimSpace(v.choice.ChapterTitle)
		return nil
	}
	ch, err := e.d.TMDB.Chapters(ctx, id.tmdbID, v.place.Season, v.place.First, v.place.Last)
	switch {
	case isNotFound(err):
		ev.warns = append(ev.warns, EditIssue{FileID: v.file.ID, File: v.file.Rel, Field: "season", Code: "unknown"})
		return nil
	case err != nil:
		ev.addIssue(v.file.ID, "chapter", "tmdb_unavailable", "")
		return nil
	}
	v.chapter = ch.Title
	if len(ch.Missing) > 0 {
		ev.warns = append(ev.warns, EditIssue{FileID: v.file.ID, File: v.file.Rel, Field: "chapter", Code: "unknown"})
	}
	return nil
}

// incomingOf returns a video as its placement needs it: where it is now, its title and place, and its
// subtitles with the languages their choices give (RF-23). A subtitle with an issue is left out.
func (e *Edits) incomingOf(id *editTitle, v *editVideo) domain.Incoming {
	in := domain.Incoming{
		Source: v.file.Rel, Kind: id.kind, Root: id.root, Title: id.title, Place: v.place,
		ChapterTitle: v.chapter, Extension: extOf(v.file.Rel),
	}
	for _, s := range v.subs {
		if !s.chosen || s.bad || s.language == "" {
			continue
		}
		in.Subtitles = append(in.Subtitles, domain.IncomingSubtitle{
			Source: s.file.Rel, Language: s.language, Forced: s.choice.Forced, SDH: s.choice.SDH, Extension: extOf(s.file.Rel),
		})
	}
	return in
}

// planLayout checks the places of the title's videos together (RF-22): a part and a whole movie of one title, two
// parts with one number (all the movie's videos), and the chapters that a moving chapter takes (movers only). The
// issues go to the videos they are about.
func (ev *editEval) planLayout() {
	var items []domain.LayoutItem
	for _, v := range ev.videos {
		if v.bad || !v.chosen {
			continue
		}
		// A series chapter that stays is a duplicate of the chapter a mover takes (RF-24), not a layout clash.
		if ev.id.kind == domain.KindSeries && !v.mover() {
			continue
		}
		items = append(items, domain.LayoutItem{File: v.file.Rel, Folder: ev.id.folder, Kind: ev.id.kind, Place: v.place})
	}
	for file, code := range domain.CheckEditLayout(items) {
		for _, v := range ev.videos {
			if v.file.Rel == file {
				ev.addIssue(v.file.ID, "place", code, "")
			}
		}
	}
}

// groupPlaces groups the videos by the place they take: the parts of a movie share one, a whole movie or a
// chapter stands alone (RF-22, RF-24). Only the groups that move a file need a decision on their duplicates.
func (ev *editEval) groupPlaces() {
	byKey := map[string]*editGroup{}
	for _, v := range ev.videos {
		if v.bad || !v.chosen || len(v.moves) == 0 {
			continue
		}
		folder := strings.ToLower(ev.id.folder)
		var key string
		switch {
		case ev.id.kind == domain.KindMovie && v.place.Part > 0:
			key = "parts|" + folder
		case ev.id.kind == domain.KindMovie:
			key = "whole|" + folder + "|" + strings.ToLower(v.file.Rel)
		default:
			key = fmt.Sprintf("episode|%s|%d|%d|%d|%s", folder, v.place.Season, v.place.First, v.place.Last, strings.ToLower(v.file.Rel))
		}
		g := byKey[key]
		if g == nil {
			g = &editGroup{key: key}
			byKey[key] = g
			ev.groups = append(ev.groups, g)
		}
		g.members = append(g.members, v)
		v.group = g
	}
}

// mover reports whether the video changes its path (RF-44): only those leave the duplicates' search.
func (v *editVideo) mover() bool {
	return !strings.EqualFold(v.finalOfVideo(), v.file.Rel)
}

// hasMover reports whether one of the group's videos changes its path.
func (g *editGroup) hasMover() bool {
	for _, m := range g.members {
		if m.mover() {
			return true
		}
	}
	return false
}

// movers returns the videos that change their paths: their old paths leave the duplicates' search (RF-44).
func (ev *editEval) movers() map[string]bool {
	out := map[string]bool{}
	for _, v := range ev.videos {
		if v.bad || !v.chosen || len(v.moves) == 0 {
			continue
		}
		if v.mover() {
			out[strings.ToLower(v.file.Rel)] = true
		}
	}
	return out
}

// folderChanged reports whether the title's folder changes: then every file of the title moves, and no file
// may stay behind (its title would not be in its folder any more).
func (ev *editEval) folderChanged() bool {
	return !strings.EqualFold(ev.id.folder, ev.st.title.Folder)
}

// checkFinals refuses two files that would take one path (RF-25), after the decisions of the duplicates.
func (ev *editEval) checkFinals() {
	seen := map[string]bool{}
	for _, v := range ev.videos {
		if v.bad || !v.chosen {
			continue
		}
		key := strings.ToLower(v.finalOfVideo())
		if seen[key] {
			ev.addIssue(v.file.ID, "place", "duplicate_target", "")
		}
		seen[key] = true
	}
}

// finish builds the plan from the decisions: where each file ends, the duplicates and the answers they accept,
// and the errors and warnings. Valid is true when the same request can be committed.
func (ev *editEval) finish(st editState, id *editTitle) {
	plan := EditPlan{TitleID: st.title.ID, Revision: st.revision, Kind: st.title.Kind, Title: st.title.DiscTitle, Folder: st.title.Folder}
	if id != nil {
		plan.Kind, plan.Title, plan.Folder = id.kind, id.display, id.folder
		if id.merge != nil {
			plan.MergeInto = id.merge.ID
		}
	}
	for _, v := range ev.videos {
		final := v.file.Rel
		if !v.bad && v.chosen {
			final = v.finalOfVideo()
		}
		ev.moves = append(ev.moves, movesOfNamesFor(v.moves)...)
		pv := EditedVideo{FileID: v.file.ID, File: v.file.Rel, FinalPath: final, Changed: !strings.EqualFold(final, v.file.Rel), Decision: v.decision, Version: v.version}
		if v.hasDups {
			pv.Duplicates, pv.ExactCopy, pv.Allowed, pv.Proposal = previewDuplicates(v, ev.folderChanged())
			pv.VersionProposal, pv.VersionIncomplete = v.labelProp, v.labelBad
		}
		plan.Videos = append(plan.Videos, pv)
	}
	for _, s := range ev.subs {
		final := s.file.Rel
		if s.video != nil && !s.video.bad && s.video.chosen && !s.video.skipped {
			final = finalPath(s.video.moves, s.file.Rel)
		}
		plan.Subtitles = append(plan.Subtitles, EditedSubtitle{FileID: s.file.ID, File: s.file.Rel, FinalPath: final, Changed: !strings.EqualFold(final, s.file.Rel)})
	}
	for _, rel := range ev.labelsGiven() {
		ev.addIssue(0, "version", "unknown", rel)
	}
	ev.issues = appendUnique(ev.issues)
	plan.Errors = ev.issues
	plan.Warnings = ev.warns
	plan.Valid = len(ev.issues) == 0
	ev.plan = plan
}

// movesOfNamesFor returns the moves of a file group that really change a path (From != To).
func movesOfNamesFor(moves []domain.Move) []domain.Move {
	var out []domain.Move
	for _, m := range moves {
		if !strings.EqualFold(m.From, m.To) {
			out = append(out, m)
		}
	}
	return out
}

// previewDuplicates returns what the preview shows of a video's duplicates: each one, whether they are an exact
// copy (RF-24), the answers they accept, and the proposal. A title whose folder changes accepts no Omitir.
func previewDuplicates(v *editVideo, folderChanged bool) ([]PlannedDuplicate, bool, []domain.Decision, domain.Decision) {
	var out []PlannedDuplicate
	for i, o := range v.occ {
		d := PlannedDuplicate{Rel: o.Video.Rel, Size: o.Video.Size, Version: o.Video.Version}
		if i < len(v.occLabelsOf()) {
			d.VersionProposal = v.occLabelsOf()[i].Text
			d.VersionNeeded = o.Video.Version == "" && v.occLabelsOf()[i].Incomplete
		}
		out = append(out, d)
	}
	allowed := allowedFor(v.dups.Exclusive || unknownVersion(v.occ))
	if folderChanged {
		allowed = withoutSkip(allowed)
	}
	var proposal domain.Decision
	if v.dups.ExactCopy {
		proposal = domain.DecisionSkip
	}
	return out, v.dups.ExactCopy, allowed, proposal
}

// occLabelsOf returns the labels the rule proposes for the versions of a video's place.
func (v *editVideo) occLabelsOf() []domain.VersionLabel {
	return v.occLabels
}

// withoutSkip returns the answers without Omitir.
func withoutSkip(in []domain.Decision) []domain.Decision {
	var out []domain.Decision
	for _, d := range in {
		if d != domain.DecisionSkip {
			out = append(out, d)
		}
	}
	return out
}

// labelsGiven returns the labels of the request that no version addressed, so that the plan says so (RF-24).
func (ev *editEval) labelsGiven() []string {
	var out []string
	for _, rel := range ev.labelRels {
		if !ev.labelUsed[strings.ToLower(rel)] {
			out = append(out, rel)
		}
	}
	return out
}

// appendUnique returns the issues without repetitions (the same file, field, code and target).
func appendUnique(issues []EditIssue) []EditIssue {
	seen := map[EditIssue]bool{}
	var out []EditIssue
	for _, is := range issues {
		if seen[is] {
			continue
		}
		seen[is] = true
		out = append(out, is)
	}
	return out
}
