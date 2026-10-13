package domain

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// The decisions on a duplicate (spec RF-24) as plans of a library change (RNF-04). A plan says which
// file moves where: the replaced files to the trash with their subtitles, the kept versions to their
// labelled names, and the new videos and subtitles to their final paths. Nothing is overwritten: the
// plan never picks a name that a file of the library holds (the engine refuses one that does). Pure:
// no I/O. The application runs a plan through the engine (Engine.Decide).

// Decision is what the user answers to a duplicate (RF-24).
type Decision string

// The decisions of a duplicate.
const (
	// DecisionReplace sends the files of the place to the trash, with their subtitles.
	DecisionReplace Decision = "replace"
	// DecisionKeepBoth keeps the files of the place as versions, with their labels (RF-24).
	DecisionKeepBoth Decision = "keep_both"
	// DecisionSkip leaves everything as it is: the new file stays in No asignados.
	DecisionSkip Decision = "skip"
)

// The reasons a decision cannot be planned.
var (
	// ErrNoDuplicate means there is no file of the place to decide on.
	ErrNoDuplicate = errors.New("there is no duplicate to decide on")
	// ErrExclusive means a version cannot be kept: parts and versions are exclusive (RF-22), so only
	// Replace or Skip is possible.
	ErrExclusive = errors.New("parts and versions are exclusive: only Replace or Skip is possible")
	// ErrVersionUnknown means a file of the place has no readable version (RF-24): it is not
	// registered and its name does not say it. Keeping both waits for its import (RF-29).
	ErrVersionUnknown = errors.New("the version of a file of the place is not known")
	// ErrIncompleteLabel means the label of a version cannot be proposed yet: the user completes it
	// in the preview, and the plan takes it from there (RF-24).
	ErrIncompleteLabel = errors.New("the label of a version is incomplete: the user completes it")
	// ErrInvalidVersion means a label the user wrote has nothing usable in it once cleaned (RF-24).
	ErrInvalidVersion = errors.New("the version label is empty once cleaned")
)

// Occupant is a video that holds the place a new video is going to take, with the subtitles that go
// with it (RF-23, RF-24): they are renamed or trashed with their video.
type Occupant struct {
	Video Existing
	// Subtitles are the subtitles of the video: the files of its folder whose name is the video's
	// name, then a dot and the language, the flags and the extension.
	Subtitles []Existing
	// Media is what its label reads from the file (ffprobe).
	Media VersionMedia
	// Label is the version the user wrote for this video, when it has no readable label of its own (RF-24).
	// It is "" when the user wrote none.
	Label string
}

// IncomingSubtitle is a subtitle of a new video, where it waits and what it is (RF-23).
type IncomingSubtitle struct {
	Source    string
	Language  string
	Forced    bool
	SDH       bool
	Extension string
}

// Incoming is a new video of an assignment, with the subtitles that go with it (RF-22, RF-23).
type Incoming struct {
	// Source is where the video waits now, in the library (_unassigned/).
	Source string
	Kind   TitleKind
	// Root is MOVIES_DIR or SERIES_DIR; Title is the title the file is named after (spec §5).
	Root  string
	Title Title
	// Place is the part of a movie or the chapters of a series.
	Place Place
	// ChapterTitle is <título cap.> of a series file; "" when it has none.
	ChapterTitle string
	// Extension is the video's extension, with its dot, in any letter case.
	Extension string
	Media     VersionMedia
	Subtitles []IncomingSubtitle
	// Version is the label the user chose in the preview (RF-24, editable); "" takes the proposed one.
	Version string
}

// Move is a file that a decision takes from one path to another. A move with an empty To is a file
// that goes to the trash.
type Move struct {
	// ID is the registry's id of the file; 0 for a file only the disk has.
	ID   int64
	From string
	To   string
	Size int64
	// Version is the label a renamed video takes (RF-24); "" for the other moves.
	Version string
}

// DecisionPlan is what a decision does to the library, step by step (RF-24).
type DecisionPlan struct {
	Decision Decision
	// Trash are the replaced files, each video first and then its subtitles (Replace only).
	Trash []Move
	// Renames are the files kept as versions that take their label (Keep both): a version with no
	// label yet, and its subtitles.
	Renames []Move
	// Place are the new videos and their subtitles, from where they wait to their final paths.
	Place []Move
	// Version is the label of the new video (Keep both); "" for the other decisions.
	Version string
}

// PlanDecision plans a decision on the videos that hold a place (RF-24). occupants are the files of
// the place, exclusive says whether versions are excluded (a part is involved, RF-22), and incoming is
// the new video (one, unless parts of a movie, which Replace takes whole).
func PlanDecision(decision Decision, occupants []Occupant, exclusive bool, incoming []Incoming) (DecisionPlan, error) {
	plan := DecisionPlan{Decision: decision}
	switch decision {
	case DecisionSkip:
		return plan, nil
	case DecisionReplace:
		if len(occupants) == 0 {
			return DecisionPlan{}, ErrNoDuplicate
		}
		plan.Trash = trashMoves(occupants)
		for _, in := range incoming {
			if in.Version != "" {
				return DecisionPlan{}, fmt.Errorf("%w: a replacing video takes no version", ErrInvalidLibrary)
			}
			moves, err := placeIncoming(in, "")
			if err != nil {
				return DecisionPlan{}, err
			}
			plan.Place = append(plan.Place, moves...)
		}
		return plan, nil
	case DecisionKeepBoth:
		if len(occupants) == 0 {
			return DecisionPlan{}, ErrNoDuplicate
		}
		if exclusive {
			return DecisionPlan{}, ErrExclusive
		}
		if len(incoming) != 1 {
			return DecisionPlan{}, fmt.Errorf("%w: keeping both takes one new video", ErrInvalidLibrary)
		}
		return planKeepBoth(plan, occupants, incoming[0])
	default:
		return DecisionPlan{}, fmt.Errorf("%w: decision %q", ErrInvalidLibrary, decision)
	}
}

// trashMoves returns the files of the occupants that go to the trash: each video, then its subtitles.
func trashMoves(occupants []Occupant) []Move {
	var out []Move
	for _, o := range occupants {
		out = append(out, Move{ID: o.Video.ID, From: o.Video.Rel, Size: o.Video.Size})
		for _, s := range o.Subtitles {
			out = append(out, Move{ID: s.ID, From: s.Rel, Size: s.Size})
		}
	}
	return out
}

// planKeepBoth plans the versions of a place (RF-24): the labels are computed over all the versions.
// An occupant with no label takes the label that the user wrote for it, or the one the rule proposes
// when that one is complete; it is refused when neither is usable. The occupants that have a label keep
// it, and the new video takes the label the user wrote or the one computed for it. A label that another
// version of the place already has gets " (2)", " (3)"…
func planKeepBoth(plan DecisionPlan, occupants []Occupant, in Incoming) (DecisionPlan, error) {
	labels := ProposeVersions(occupants, in)

	taken := make([]string, 0, len(occupants)+1)
	for _, o := range occupants {
		if !o.Video.VersionKnown {
			return DecisionPlan{}, ErrVersionUnknown
		}
		if o.Video.Version != "" {
			taken = append(taken, o.Video.Version)
		}
	}
	for i, o := range occupants {
		if o.Video.Version != "" {
			continue
		}
		version, err := occupantVersion(o, labels[i])
		if err != nil {
			return DecisionPlan{}, err
		}
		version = uniqueVersion(version, taken)
		renames, err := renameWithVersion(o, version)
		if err != nil {
			return DecisionPlan{}, err
		}
		plan.Renames = append(plan.Renames, renames...)
		taken = append(taken, version)
	}

	proposed := labels[len(occupants)]
	version := proposed.Text
	if in.Version != "" {
		clean, err := CleanVersion(in.Version)
		if err != nil {
			return DecisionPlan{}, err
		}
		version = clean
	} else if proposed.Incomplete {
		return DecisionPlan{}, ErrIncompleteLabel
	}
	version = uniqueVersion(version, taken)
	moves, err := placeIncoming(in, version)
	if err != nil {
		return DecisionPlan{}, err
	}
	plan.Place = append(plan.Place, moves...)
	plan.Version = version
	return plan, nil
}

// occupantVersion returns the label that an occupant without one takes when both versions are kept: the
// label the user wrote for it, cleaned as a label of a new version is (RF-24), or else the proposed one
// when it is complete.
func occupantVersion(o Occupant, proposed VersionLabel) (string, error) {
	if o.Label != "" {
		return CleanVersion(o.Label)
	}
	if proposed.Incomplete {
		return "", ErrIncompleteLabel
	}
	return proposed.Text, nil
}

// CleanVersion returns a label that the user wrote, as it goes in a file name (spec §5, steps 1 to 6).
// It fails with ErrInvalidVersion when nothing usable is left, for example a label of dots and spaces.
func CleanVersion(label string) (string, error) {
	v := finishName(cleanChars(label))
	if v == "" {
		return "", ErrInvalidVersion
	}
	return v, nil
}

// renameWithVersion returns the moves that give an occupant with no label its version (RF-24): its
// video and its subtitles take the label after their names, as spec §5 writes the <versión>.
func renameWithVersion(o Occupant, version string) ([]Move, error) {
	dir, name := path.Split(o.Video.Rel)
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	newStem := stem + " - " + version
	if len(newStem)+len(ext) > MaxNameBytes {
		return nil, &NameError{Field: "version"}
	}
	out := []Move{{ID: o.Video.ID, From: o.Video.Rel, To: dir + newStem + ext, Size: o.Video.Size, Version: version}}
	for _, s := range o.Subtitles {
		suffix, ok := SubtitleSuffix(o.Video.Rel, s.Rel)
		if !ok {
			return nil, fmt.Errorf("%w: subtitle %s does not name its video", ErrInvalidLibrary, s.Rel)
		}
		newName := newStem + suffix
		if len(newName) > MaxNameBytes {
			return nil, &NameError{Field: "version"}
		}
		out = append(out, Move{ID: s.ID, From: s.Rel, To: dir + newName, Size: s.Size})
	}
	return out, nil
}

// PlanPlacement plans the placement of new videos that take no place with anything in the library
// (no duplicate, RF-24): each video and its subtitles go to their final paths, without a version.
// The plan has no decision.
func PlanPlacement(incoming []Incoming) (DecisionPlan, error) {
	var plan DecisionPlan
	for _, in := range incoming {
		if in.Version != "" {
			return DecisionPlan{}, fmt.Errorf("%w: a video with no duplicate takes no version", ErrInvalidLibrary)
		}
		moves, err := placeIncoming(in, "")
		if err != nil {
			return DecisionPlan{}, err
		}
		plan.Place = append(plan.Place, moves...)
	}
	return plan, nil
}

// ProposeVersions returns the label that each occupant of a place would take as a version, and then the
// one of the new video (RF-24), in that order. The labels are computed over all of them, so an incomplete
// one is reported with Incomplete set and the user completes it.
func ProposeVersions(occupants []Occupant, in Incoming) []VersionLabel {
	media := make([]VersionMedia, 0, len(occupants)+1)
	for _, o := range occupants {
		media = append(media, o.Media)
	}
	media = append(media, in.Media)
	return VersionLabels(media)
}

// ProposeVersion returns the label that a new video would take among the occupants of its place.
func ProposeVersion(occupants []Occupant, in Incoming) VersionLabel {
	return ProposeVersions(occupants, in)[len(occupants)]
}

// placeIncoming returns the moves that place a new video and its subtitles at their final paths
// (spec §5), with version as its <versión>. The video goes first.
func placeIncoming(in Incoming, version string) ([]Move, error) {
	var (
		video string
		err   error
	)
	switch in.Kind {
	case KindMovie:
		video, err = MoviePath(Movie{Root: in.Root, Title: in.Title, Version: version, Part: in.Place.Part, Extension: in.Extension})
	case KindSeries:
		video, err = EpisodePath(Episode{
			Root: in.Root, Title: in.Title, Season: in.Place.Season, First: in.Place.First, Last: in.Place.Last,
			ChapterTitle: in.ChapterTitle, Version: version, Extension: in.Extension,
		})
	default:
		return nil, fmt.Errorf("%w: kind %q", ErrInvalidLibrary, in.Kind)
	}
	if err != nil {
		return nil, err
	}
	moves := []Move{{From: in.Source, To: video}}
	for _, s := range in.Subtitles {
		final, err := SubtitlePath(video, Subtitle{Language: s.Language, Forced: s.Forced, SDH: s.SDH, Extension: s.Extension})
		if err != nil {
			return nil, err
		}
		moves = append(moves, Move{From: s.Source, To: final})
	}
	return moves, nil
}

// uniqueVersion returns version, or the first of " (2)", " (3)"… that no taken version has (RF-24).
func uniqueVersion(version string, taken []string) string {
	if !containsFold(taken, version) {
		return version
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)", version, n)
		if !containsFold(taken, candidate) {
			return candidate
		}
	}
}

// containsFold reports whether one of the names is name, in any letter case.
func containsFold(names []string, name string) bool {
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// SubtitleSuffix returns what follows the video's name in its subtitle's name: ".en.forced.srt" for
// "Movie.en.forced.srt" next to "Movie.mkv". ok is false when the subtitle's name does not start with
// the video's name and a dot.
func SubtitleSuffix(videoRel, subtitleRel string) (string, bool) {
	videoStem := stemOf(path.Base(videoRel))
	name := path.Base(subtitleRel)
	if !strings.HasPrefix(name, videoStem+".") {
		return "", false
	}
	return name[len(videoStem):], true
}

// MovieVersionOf reads the version of a movie's file from its name (RF-24, RF-29): a file named as its
// folder is the whole movie, with no version, and "<folder name> - <version>" has the version after the
// folder's name. ok is false for any other name: the version cannot be read.
func MovieVersionOf(folder, rel string) (string, bool) {
	base := path.Base(folder)
	stem := stemOf(path.Base(rel))
	switch {
	case stem == base:
		return "", true
	case partNamePattern.MatchString(stem) && strings.HasPrefix(stem, base+" - "):
		return "", true // a part is not a version (RF-22)
	case strings.HasPrefix(stem, base+" - "):
		version := stem[len(base)+len(" - "):]
		if !plainName(version) {
			return "", false
		}
		return version, true
	default:
		return "", false
	}
}

// chapterEndPattern matches a chapter code at the end of a series file's name: nothing follows it, so
// the file has no title and no version.
var chapterEndPattern = regexp.MustCompile(`(?i) - S\d+E\d+(?:-E\d+)?$`)

// SeriesVersionOf reads the version of a chapter's file from its name (RF-24, RF-29): a file named with
// only its chapter code has no version. Any other name may hold a title, a version, or both, and the
// name alone does not tell which: its version cannot be read (ok is false) until the file is imported.
func SeriesVersionOf(rel string) (string, bool) {
	if chapterEndPattern.MatchString(stemOf(path.Base(rel))) {
		return "", true
	}
	return "", false
}

// stemOf returns a file name without its last extension.
func stemOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}
