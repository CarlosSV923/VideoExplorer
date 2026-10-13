package domain

import (
	"path"
	"slices"
	"strings"
	"time"
)

// SettleTime is how long a file must stay unmodified before the section takes it as complete (RF-27).
const SettleTime = time.Minute

// Statuses of an entry of the section (RF-27): copying while one of its files is still being copied,
// assigning while an assignment holds it (RF-26), ready otherwise.
const (
	StatusReady     = "ready"
	StatusCopying   = "copying"
	StatusAssigning = "assigning"
)

// SafeEntryPath reports whether s is a path inside an entry of the section: one or more names joined with
// "/", none of them empty, "." or "..", and no separator or control character in a name.
func SafeEntryPath(s string) bool {
	return safeRelativePath(s)
}

// Item is something inside an entry of the section, as the disk holds it now.
type Item struct {
	// Rel is its path inside the entry, with "/" separators; "" is a loose entry's own file.
	Rel     string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// Folder is an entry of the section as the disk holds it: a first-level folder, or a loose file.
type Folder struct {
	Name  string
	Loose bool
	// Items is everything inside the entry, junk included, so that the junk can go with it.
	Items []Item
}

// Stamp is what a reading keeps of a file: its size and its modification time (RF-27).
type Stamp struct {
	Size    int64
	ModTime time.Time
}

// Copying reports whether a file is still being copied: it was modified less than SettleTime ago, or
// it changed since the previous reading (prev, nil when there was none). The times are compared to the
// millisecond, the precision the registry keeps. A modification time in the future (a clock that
// differs from the NAS) does not count as recent: it would keep the file in copy for good.
func Copying(prev *Stamp, cur Stamp, now time.Time) bool {
	if age := now.Sub(cur.ModTime); age >= 0 && age < SettleTime {
		return true
	}
	return prev != nil && (prev.Size != cur.Size || prev.ModTime.UnixMilli() != cur.ModTime.UnixMilli())
}

// Ignored reports whether a path inside _unassigned/ is junk (RF-28): a name that starts with '.', '@',
// '#' or '$' in any of its folders or in its name, and Thumbs.db or desktop.ini in any letter case. The
// section does not list or count junk (RF-27), and it goes without the trash when its entry is emptied.
func Ignored(rel string) bool {
	for seg := range strings.SplitSeq(rel, "/") {
		if strings.HasPrefix(seg, ".") || strings.HasPrefix(seg, "@") || strings.HasPrefix(seg, "#") || strings.HasPrefix(seg, "$") {
			return true
		}
		switch strings.ToLower(seg) {
		case "thumbs.db", "desktop.ini":
			return true
		}
	}
	return false
}

// EntryStatus returns the status of an entry: copying while a file is still being copied, otherwise
// assigning while its assignment lock is held, otherwise ready.
func EntryStatus(copying, assigning bool) string {
	switch {
	case copying:
		return StatusCopying
	case assigning:
		return StatusAssigning
	default:
		return StatusReady
	}
}

// Entries groups everything inside _unassigned/ (as the disk lists it) into the entries of the section:
// the first-level names that are not junk, each a folder or a loose file (RF-27). The result is sorted
// by name, in any letter case.
func Entries(tree []Item) []Folder {
	byName := map[string]*Folder{}
	var names []string
	for _, it := range tree {
		top, rest, nested := strings.Cut(it.Rel, "/")
		if top == "" || Ignored(top) {
			continue
		}
		if !nested {
			if it.IsDir {
				byName[top] = &Folder{Name: top}
			} else {
				byName[top] = &Folder{Name: top, Loose: true, Items: []Item{{Rel: "", Size: it.Size, ModTime: it.ModTime}}}
			}
			names = append(names, top)
			continue
		}
		f := byName[top]
		if f == nil || f.Loose {
			continue
		}
		f.Items = append(f.Items, Item{Rel: rest, IsDir: it.IsDir, Size: it.Size, ModTime: it.ModTime})
	}
	out := make([]Folder, 0, len(names))
	for _, n := range names {
		out = append(out, *byName[n])
	}
	slices.SortFunc(out, func(a, b Folder) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out
}

// Files returns the items of an entry that the section lists: the files that are not junk, and a
// loose entry's own file (RF-27). Folders are not listed.
func Files(items []Item) []Item {
	var out []Item
	for _, it := range items {
		if !it.IsDir && !Ignored(it.Rel) {
			out = append(out, it)
		}
	}
	return out
}

// Vacate works out what leaves a folder of the section once the items in gone have left it (RF-27).
// When the entry keeps something, a folder goes only when everything in it has gone: junk keeps it, and
// junk is not touched. When the entry is left empty, what remains is junk and folders: the outermost
// junk items go (the caller deletes them without the trash) and every folder goes, deepest first. gone
// holds the rels of the files and folders that leave; it is not changed. Both lists are sorted, the
// folders deepest first.
func Vacate(items []Item, gone map[string]bool, empty bool) (junk, folders []string) {
	if empty {
		for _, it := range items {
			// Only the outermost junk: an ignored item inside an ignored folder goes with that folder.
			if gone[it.Rel] || !Ignored(it.Rel) || Ignored(parentOf(it.Rel)) {
				continue
			}
			junk = append(junk, it.Rel)
		}
		for _, it := range items {
			if it.IsDir && !Ignored(it.Rel) {
				folders = append(folders, it.Rel)
			}
		}
		slices.Sort(junk)
		sortDeepestFirst(folders)
		return junk, folders
	}

	// The folders that hold a leaving item are candidates; each goes when its children have all gone,
	// deepest first, so that a folder that empties makes its own parent a candidate in turn.
	removed := map[string]bool{}
	for rel := range gone {
		removed[rel] = true
	}
	children := map[string][]string{}
	candidates := map[string]bool{}
	for _, it := range items {
		parent := parentOf(it.Rel)
		children[parent] = append(children[parent], it.Rel)
		if gone[it.Rel] {
			for d := parent; d != ""; d = parentOf(d) {
				candidates[d] = true
			}
		}
	}
	order := make([]string, 0, len(candidates))
	for d := range candidates {
		if !Ignored(d) {
			order = append(order, d)
		}
	}
	sortDeepestFirst(order)
	for _, d := range order {
		if removed[d] {
			continue
		}
		all := true
		for _, c := range children[d] {
			if !removed[c] {
				all = false
				break
			}
		}
		if all {
			removed[d] = true
			folders = append(folders, d)
		}
	}
	return nil, folders
}

// parentOf returns the folder that holds rel inside its entry: "" for the entry itself.
func parentOf(rel string) string {
	if rel == "" {
		return ""
	}
	if p := path.Dir(rel); p != "." {
		return p
	}
	return ""
}

// sortDeepestFirst sorts folder rels so that a folder comes before the folders that are its parents.
func sortDeepestFirst(dirs []string) {
	slices.SortFunc(dirs, func(a, b string) int {
		if c := strings.Count(b, "/") - strings.Count(a, "/"); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
}
