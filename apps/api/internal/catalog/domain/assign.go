package domain

import (
	"strings"
	"time"
)

// Assignment rules (spec RF-20 to RF-26) that need no I/O: the lock an open assignment holds
// on an entry of No asignados, the place a chosen video takes, and the conflicts between the
// videos chosen for one entry. The application reads the titles and the disk; these functions
// only decide.

// AssignLockTTL is how long an assignment holds its entry without activity (RF-26): every plan
// renews the lock, and a tab that is closed frees it when this time has passed.
const AssignLockTTL = 15 * time.Minute

// AssignLock is the lock an open assignment holds on an entry (RF-26). Token identifies the tab
// that holds it; Until is when it expires unless renewed. The zero value is a free entry.
type AssignLock struct {
	Token string
	Until time.Time
}

// Held reports whether the lock is taken at now.
func (l AssignLock) Held(now time.Time) bool {
	return l.Token != "" && now.Before(l.Until)
}

// HeldBy reports whether the lock is taken at now by token.
func (l AssignLock) HeldBy(token string, now time.Time) bool {
	return token != "" && l.Held(now) && l.Token == token
}

// Issue is one thing that a choice gets wrong: the field it names, and a stable code the UI
// translates (RF-24, RF-22).
type Issue struct {
	Field string
	Code  string
}

// PlaceOf checks the numbers that say where a chosen video goes in its title (RF-22) and returns
// the place. A movie takes at most a part number (0 is the whole movie); a series takes a season
// (0 holds the specials) and a chapter, and optionally the last chapter of a file with several.
// It returns the first issue found, or a zero Issue when the place is valid.
func PlaceOf(kind TitleKind, part int, season, chapter, last *int) (Place, Issue) {
	switch kind {
	case KindMovie:
		if season != nil || chapter != nil || last != nil {
			return Place{}, Issue{Field: "place", Code: "kind_mismatch"}
		}
		if part < 0 || part > maxPart {
			return Place{}, Issue{Field: "part", Code: "invalid"}
		}
		return Place{Part: part}, Issue{}
	case KindSeries:
		if part != 0 {
			return Place{}, Issue{Field: "place", Code: "kind_mismatch"}
		}
		if season == nil {
			return Place{}, Issue{Field: "season", Code: "required"}
		}
		if *season < 0 || *season > maxNumber {
			return Place{}, Issue{Field: "season", Code: "invalid"}
		}
		if chapter == nil {
			return Place{}, Issue{Field: "chapter", Code: "required"}
		}
		if *chapter < 1 || *chapter > maxNumber {
			return Place{}, Issue{Field: "chapter", Code: "invalid"}
		}
		first, end := *chapter, *chapter
		if last != nil {
			if *last < first || *last > maxNumber {
				return Place{}, Issue{Field: "lastChapter", Code: "invalid"}
			}
			end = *last
		}
		return Place{Episode: true, Season: *season, First: first, Last: end}, Issue{}
	default:
		return Place{}, Issue{Field: "title", Code: "invalid"}
	}
}

// maxPart and maxNumber bound the numbers a user types for a part, a season or a chapter.
const (
	maxPart   = 999
	maxNumber = 999
)

// LayoutItem is a video the user chose to place, with the title it goes to and its place. Folder
// is the title's folder inside the library, in any letter case.
type LayoutItem struct {
	File   string
	Folder string
	Kind   TitleKind
	Place  Place
}

// Layout codes of CheckLayout.
const (
	// CodeDuplicateTarget means two videos of one entry take the same place.
	CodeDuplicateTarget = "duplicate_target"
	// CodePartsAndWhole means a movie gets parts and a whole file in the same entry (RF-22).
	CodePartsAndWhole = "parts_and_whole"
	// CodeChaptersOverlap means two files of one entry hold the same chapter.
	CodeChaptersOverlap = "chapters_overlap"
)

// CheckLayout returns, for each item that conflicts with an earlier item of the same title, the
// code of the conflict (spec RF-22, RF-24). The first item keeps its place; later ones are the
// conflicts. Items are compared in the order given.
func CheckLayout(items []LayoutItem) map[string]string {
	out := map[string]string{}
	type titleState struct {
		wholes int
		parts  map[int]bool
		// chapters holds the chapters taken by each season.
		chapters map[int][][2]int
	}
	titles := map[string]*titleState{}
	for _, it := range items {
		key := strings.ToLower(it.Folder)
		st := titles[key]
		if st == nil {
			st = &titleState{parts: map[int]bool{}, chapters: map[int][][2]int{}}
			titles[key] = st
		}
		switch it.Kind {
		case KindMovie:
			if it.Place.Part == 0 {
				if st.wholes > 0 || len(st.parts) > 0 {
					out[it.File] = conflictCode(st.wholes > 0, len(st.parts) > 0)
				}
				st.wholes++
				continue
			}
			if st.wholes > 0 {
				out[it.File] = CodePartsAndWhole
				continue
			}
			if st.parts[it.Place.Part] {
				out[it.File] = CodeDuplicateTarget
				continue
			}
			st.parts[it.Place.Part] = true
		case KindSeries:
			p := it.Place
			overlaps := false
			for _, r := range st.chapters[p.Season] {
				if p.First <= r[1] && r[0] <= p.Last {
					overlaps = true
					break
				}
			}
			if overlaps {
				out[it.File] = CodeChaptersOverlap
				continue
			}
			st.chapters[p.Season] = append(st.chapters[p.Season], [2]int{p.First, p.Last})
		}
	}
	return out
}

// conflictCode names the conflict of a whole movie that comes after parts or another whole movie.
func conflictCode(hasWhole, hasParts bool) string {
	if hasParts && !hasWhole {
		return CodePartsAndWhole
	}
	return CodeDuplicateTarget
}
