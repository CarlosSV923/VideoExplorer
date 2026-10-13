package domain

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Duplicates (spec RF-22, RF-24): what already holds the place a new video is going to take.
// The place is a movie, as a whole or as its parts, or the chapters of a series. A place is
// occupied by the live videos (not in the trash) that the registry keeps for it, and by the
// videos that the disk holds in the title's folder even when the registry does not know them.
// Subtitles are never duplicates: they go with their video. Pure: no I/O.

// Target is the place a new video is going to take in its title (RF-22).
type Target struct {
	Kind TitleKind
	// Parts is true for a movie whose new content is a set of parts (RF-22). The place is then
	// the whole movie: a duplicate of a part or of the whole movie is a duplicate of the movie
	// (RF-24), and parts and versions are exclusive.
	Parts bool
	// Place is the chapters the new video holds. It is used for a series, and must be an episode.
	Place Place
}

// Existing is a video that may hold the target's place: one the registry keeps, or one found
// on the disk of the title's folder (RF-24).
type Existing struct {
	Rel string
	// Place is the place of the video: its part for a movie, its chapters for a series.
	Place       Place
	Size        int64
	PartialHash string
	// Trashed is true for a file in the trash: it does not count as a duplicate (RF-24).
	Trashed bool
}

// Duplicates is what already holds the target's place (RF-24).
type Duplicates struct {
	// Files are the live videos that hold the place, once each, in the order they were given.
	Files []Existing
	// Exclusive is true when versions cannot be kept, only Replace or Skip: a part is not a
	// version (RF-22), so the place of parts, or a place that already has parts, takes no version.
	Exclusive bool
	// ExactCopy is true when a live video is the same content as the incoming one (RF-24):
	// Skip is then the proposal.
	ExactCopy bool
}

// Fingerprint is what «Copia exacta» compares: a video's size and partial hash (RF-24).
type Fingerprint struct {
	Size        int64
	PartialHash string
}

// SameContent reports whether two videos are the same content as far as the partial hash
// tells: the same size and the same partial hash, which both must have (RF-24). A video
// without a partial hash is never an exact copy.
func SameContent(a, b Fingerprint) bool {
	return a.PartialHash != "" && a.PartialHash == b.PartialHash && a.Size == b.Size
}

// FindDuplicates returns the live videos that hold the target's place, whether the registry
// keeps them or the disk has them (RF-24). existing lists the registry's files first, then the
// disk's: a file listed twice (by its path, in any letter case) counts once, and the first
// listing wins. incoming is the new video's fingerprint, for «Copia exacta».
func FindDuplicates(target Target, existing []Existing, incoming Fingerprint) (Duplicates, error) {
	if err := checkTarget(target); err != nil {
		return Duplicates{}, err
	}
	var out Duplicates
	seen := map[string]bool{}
	for _, e := range existing {
		key := strings.ToLower(e.Rel)
		if seen[key] {
			continue
		}
		seen[key] = true // a trashed file keeps its path taken: its copy on the disk is not a live file
		if e.Trashed || !holds(target, e.Place) {
			continue
		}
		out.Files = append(out.Files, e)
		if e.Place.Part > 0 {
			out.Exclusive = true
		}
		if SameContent(incoming, Fingerprint{Size: e.Size, PartialHash: e.PartialHash}) {
			out.ExactCopy = true
		}
	}
	if target.Parts && len(out.Files) > 0 {
		out.Exclusive = true
	}
	return out, nil
}

// checkTarget checks that the target describes a place its kind can have.
func checkTarget(t Target) error {
	switch t.Kind {
	case KindMovie:
		if t.Place != (Place{}) {
			return fmt.Errorf("%w: a movie target has no place of its own; its parts are given with Parts", ErrInvalidLibrary)
		}
	case KindSeries:
		if t.Parts {
			return fmt.Errorf("%w: a series has no parts", ErrInvalidLibrary)
		}
		if !t.Place.Episode || t.Place.Part != 0 || t.Place.First < 1 || t.Place.Last < t.Place.First || t.Place.Season < 0 {
			return fmt.Errorf("%w: a series target is a chapter or chapters", ErrInvalidLibrary)
		}
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidLibrary, t.Kind)
	}
	return nil
}

// holds reports whether a video at place p is in the target's place. A movie has one place: every
// video of the movie holds it. A series chapter holds the chapters it spans, in its season.
func holds(t Target, p Place) bool {
	if t.Kind == KindMovie {
		return !p.Episode
	}
	if !p.Episode {
		return false
	}
	return p.Season == t.Place.Season && p.First <= t.Place.Last && t.Place.First <= p.Last
}

// chapterCodePattern reads the code SxxEyy or SxxEyy-Ezz of a chapter file's name (spec §5).
var chapterCodePattern = regexp.MustCompile(`(?i)S(\d+)E(\d+)(?:-E(\d+))?`)

// partNamePattern reads the part number at the end of a movie file's name (spec §5, " - partN").
var partNamePattern = regexp.MustCompile(`(?i) - part(\d+)$`)

// PlaceOfName reads the place of a video found on the disk from its file name (spec §5), so
// that it can be compared with a target. A movie's file holds the whole movie, or one of its
// parts when its name ends in " - partN". A series' file must carry its chapter code, and
// ok is false for a file without one: it cannot be told apart from the other chapters.
func PlaceOfName(kind TitleKind, rel string) (p Place, ok bool) {
	name := path.Base(rel)
	stem := name
	if i := strings.LastIndex(name, "."); i > 0 {
		stem = name[:i]
	}
	switch kind {
	case KindMovie:
		if m := partNamePattern.FindStringSubmatch(stem); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil || n < 1 {
				return Place{}, false
			}
			return Place{Part: n}, true
		}
		return Place{}, true
	case KindSeries:
		m := chapterCodePattern.FindStringSubmatch(stem)
		if m == nil {
			return Place{}, false
		}
		season, err1 := strconv.Atoi(m[1])
		first, err2 := strconv.Atoi(m[2])
		last := first
		if m[3] != "" {
			var err3 error
			if last, err3 = strconv.Atoi(m[3]); err3 != nil {
				return Place{}, false
			}
		}
		if err1 != nil || err2 != nil || first < 1 || last < first {
			return Place{}, false
		}
		return Place{Episode: true, Season: season, First: first, Last: last}, true
	default:
		return Place{}, false
	}
}
