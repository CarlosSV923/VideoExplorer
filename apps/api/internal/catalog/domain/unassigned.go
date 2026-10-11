package domain

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// UnassignedDir is the folder of the unassigned section, in the library root (spec RF-27).
const UnassignedDir = "_unassigned"

// MaxNameBytes is the longest name of a file or a folder in the library (spec §5: 255 bytes).
const MaxNameBytes = 255

// Relations an unassigned entry can come with, the ones of an upload batch (spec RF-03).
const (
	RelationParts    = "parts"
	RelationEpisodes = "episodes"
	RelationNone     = "none"
)

// OriginUpload is the origin of an entry that came from an upload (spec RF-08).
const OriginUpload = "upload"

// ErrInvalidEntry is returned for an unassigned entry whose values are not usable.
var ErrInvalidEntry = errors.New("invalid unassigned entry")

// UnassignedEntry is an entry of No asignados (spec RF-08, RF-08a): a folder of
// _unassigned/ or one loose file. It is recorded together with its files once they are
// in place (the library's journal, RNF-04): a move that a failure or a restart cut is
// undone, and the content goes on from staging.
type UnassignedEntry struct {
	ID int64
	// Source is what the move is for, chosen by the caller: the batch, or the unit of a
	// batch without relation. An entry is found again by it.
	Source string
	// Name is the entry's name in _unassigned/, sanitized and without collisions (RF-08).
	Name string
	// Loose is a single video kept as it is: its name is the file name, with its extension.
	Loose bool
	// BatchID and Relation say what the entry came from; Origin is OriginUpload (RF-08a).
	BatchID   string
	Relation  string
	Origin    string
	CreatedAt time.Time
	// Files are the files of the entry, in the order they were recorded.
	Files []UnassignedFile
}

// UnassignedFile is one file of an unassigned entry, already in its place in _unassigned/.
type UnassignedFile struct {
	// Source is where the file was before it moved (its staging path).
	Source string
	// Rel is its place inside the entry, with "/" separators. Empty for a loose entry.
	Rel  string
	Size int64
}

// NewUnassignedEntry checks the values of a new entry, which has no files yet.
func NewUnassignedEntry(source, name string, loose bool, batchID, relation string, now time.Time) (UnassignedEntry, error) {
	if source == "" {
		return UnassignedEntry{}, fmt.Errorf("%w: empty source", ErrInvalidEntry)
	}
	if !safeComponent(name) || len(name) > MaxNameBytes {
		return UnassignedEntry{}, fmt.Errorf("%w: name %q", ErrInvalidEntry, name)
	}
	if !safeComponent(batchID) {
		return UnassignedEntry{}, fmt.Errorf("%w: batch %q", ErrInvalidEntry, batchID)
	}
	switch relation {
	case RelationParts, RelationEpisodes, RelationNone:
	default:
		return UnassignedEntry{}, fmt.Errorf("%w: relation %q", ErrInvalidEntry, relation)
	}
	return UnassignedEntry{
		Source: source, Name: name, Loose: loose, BatchID: batchID, Relation: relation,
		Origin: OriginUpload, CreatedAt: now,
	}, nil
}

// NewUnassignedFile checks the values of a file of an entry. A loose entry's file has
// no place of its own: its rel is empty; any other file's rel is a safe relative path.
func NewUnassignedFile(source, rel string, size int64, loose bool) (UnassignedFile, error) {
	if source == "" {
		return UnassignedFile{}, fmt.Errorf("%w: empty source", ErrInvalidEntry)
	}
	if size < 0 {
		return UnassignedFile{}, fmt.Errorf("%w: negative size", ErrInvalidEntry)
	}
	if loose {
		if rel != "" {
			return UnassignedFile{}, fmt.Errorf("%w: a loose entry has no place for its file, got %q", ErrInvalidEntry, rel)
		}
	} else if !safeRelativePath(rel) {
		return UnassignedFile{}, fmt.Errorf("%w: relative path %q", ErrInvalidEntry, rel)
	}
	return UnassignedFile{Source: source, Rel: rel, Size: size}, nil
}

// NameSet holds names compared as a Windows or SMB share compares them: without regard
// to case, so that "Movie" and "movie" are the same name.
type NameSet map[string]bool

// NewNameSet returns a set with names in it.
func NewNameSet(names ...string) NameSet {
	s := NameSet{}
	for _, n := range names {
		s.Add(n)
	}
	return s
}

// Add puts a name in the set.
func (s NameSet) Add(name string) { s[strings.ToLower(name)] = true }

// Has reports whether a name, in any letter case, is in the set.
func (s NameSet) Has(name string) bool { return s[strings.ToLower(name)] }

// FitName shortens a name that is longer than MaxNameBytes, keeping its extension when
// it is a loose file's name (spec §5). It returns the name unchanged when it fits.
func FitName(name string, loose bool) string {
	if len(name) <= MaxNameBytes {
		return name
	}
	stem, ext := splitName(name, loose)
	return shortenStem(stem, MaxNameBytes-len(ext)) + ext
}

// FreeName returns name, or the first name of " (2)", " (3)"… that is not taken (spec
// RF-08). A loose file keeps its extension after the suffix: "Movie (2).mkv".
func FreeName(name string, loose bool, taken NameSet) string {
	if !taken.Has(name) {
		return name
	}
	stem, ext := splitName(name, loose)
	for n := 2; ; n++ {
		suffix := fmt.Sprintf(" (%d)", n)
		candidate := shortenStem(stem, MaxNameBytes-len(suffix)-len(ext)) + suffix + ext
		if !taken.Has(candidate) {
			return candidate
		}
	}
}

// FreeRel returns rel, or the first rel of " (2)", " (3)"… whose file name is not taken
// in its folder (spec RF-08a): "Season 01/Pilot (2).mkv". Taken holds the rels of the entry.
func FreeRel(rel string, taken NameSet) string {
	if !taken.Has(rel) {
		return rel
	}
	dir, file := "", rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		dir, file = rel[:i+1], rel[i+1:]
	}
	stem, ext := splitName(file, true)
	for n := 2; ; n++ {
		suffix := fmt.Sprintf(" (%d)", n)
		candidate := dir + shortenStem(stem, MaxNameBytes-len(dir)-len(suffix)-len(ext)) + suffix + ext
		if !taken.Has(candidate) {
			return candidate
		}
	}
}

// splitName splits a file name into its stem and its extension. A folder's name, or a
// name with no extension (or one that is only a dot, or has a space in it), is all stem.
func splitName(name string, file bool) (stem, ext string) {
	if !file {
		return name, ""
	}
	ext = path.Ext(name)
	if len(ext) <= 1 || strings.ContainsFunc(ext, unicode.IsSpace) || ext == name {
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

// shortenStem cuts stem to at most room bytes, at a character boundary, and drops the
// spaces and dots that the cut leaves at its end (spec §5, step 6).
func shortenStem(stem string, room int) string {
	room = max(room, 0)
	if len(stem) <= room {
		return stem
	}
	cut := room
	for cut > 0 && !utf8.RuneStart(stem[cut]) {
		cut--
	}
	return strings.TrimRight(stem[:cut], " .")
}
