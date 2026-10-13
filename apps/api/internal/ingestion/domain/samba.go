package domain

import (
	"path"
	"slices"
	"strings"
)

// SambaFile is a file in No asignados that Samba copied (RF-27), as the section lists it, with what
// the adoption needs to decide whether it is taken as an upload (RF-27a).
type SambaFile struct {
	// Entry is the id of the entry of the section that holds the file.
	Entry int64
	// Rel is the file's place inside the entry, with "/" separators; "" for a loose entry, whose
	// file is the entry itself.
	Rel string
	// Name is the file's name, without folders.
	Name string
	// EntryName is the name of the entry in _unassigned/: a folder, or the loose file itself (RF-27).
	EntryName string
	// Size is its size in bytes.
	Size int64
	// Source is where the file is now. It is built by the server and never leaves it.
	Source string
	// Copying says the file is still being copied (RF-27): it is not taken until it settles.
	Copying bool
	// Archive says the file starts with the magic bytes of a supported archive (RF-04), read by
	// the adoption; the extension is never trusted.
	Archive bool
}

// dir is the folder of the file inside its entry, "" at the top of the entry.
func (f SambaFile) dir() string {
	d := path.Dir(f.Rel)
	if d == "." {
		return ""
	}
	return d
}

// SambaUnit is what one batch takes from the copies of Samba (RF-27a): one archive, the volumes of a
// multi-volume archive, or an old-style RAR with its old volumes. Its files are in one folder of one
// entry.
type SambaUnit struct {
	// Entry is the entry of the section that holds every file of the unit.
	Entry int64
	// Files are the unit's files. A set is in volume order; an old-style RAR comes first, then its old
	// volumes by name; a single file is alone.
	Files []SambaFile
	// Set says the files are the volumes of a multi-volume archive (RF-03). Their total is len(Files),
	// the number of volumes found: a hole among them is for the extraction to report (RF-27a).
	Set bool
}

// GroupSamba decides which copies Samba made are taken now, and groups them into units (RF-27a). A
// file is taken when it is settled (not copying) and not empty, and it is either an archive by its
// magic bytes or an old-style RAR volume by its name (RF-03). The volumes of one set in one folder
// form one unit when the first volume is an archive; a plain .rar that is an archive forms one unit
// with the old volumes of its name. A set of more than MaxPartsTotal volumes, or one with a file still
// being copied, waits as a whole and none of its volumes is taken alone. Everything else stays where
// it is. The units come sorted by entry and by path, so the work is the same on every run.
func GroupSamba(files []SambaFile) []SambaUnit {
	type folder struct {
		entry int64
		dir   string
	}
	byFolder := map[folder][]SambaFile{}
	var order []folder
	for _, f := range files {
		k := folder{f.Entry, f.dir()}
		if _, seen := byFolder[k]; !seen {
			order = append(order, k)
		}
		byFolder[k] = append(byFolder[k], f)
	}
	var out []SambaUnit
	for _, k := range order {
		out = append(out, groupFolder(k.entry, byFolder[k])...)
	}
	slices.SortFunc(out, func(a, b SambaUnit) int {
		if a.Entry != b.Entry {
			if a.Entry < b.Entry {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Files[0].Rel, b.Files[0].Rel)
	})
	return out
}

// groupFolder groups the files of one folder of one entry. Each file is claimed by at most one unit.
func groupFolder(entry int64, files []SambaFile) []SambaUnit {
	claimed := make([]bool, len(files))
	var out []SambaUnit

	// Volumes of multi-volume archives, one set per name (RF-03). A set that is not taken claims its
	// volumes all the same, so that none of them is taken alone.
	sets := map[string][]int{}
	var setOrder []string
	for i, f := range files {
		v, ok := ParseVolume(f.Name)
		if !ok {
			continue
		}
		if _, seen := sets[v.Set]; !seen {
			setOrder = append(setOrder, v.Set)
		}
		sets[v.Set] = append(sets[v.Set], i)
		claimed[i] = true
	}
	for _, set := range setOrder {
		members := sortedByVolume(files, sets[set])
		if len(members) > MaxPartsTotal || !isArchiveVolume(files, members) || !settled(files, members) {
			continue
		}
		out = append(out, SambaUnit{Entry: entry, Files: pick(files, members), Set: true})
	}

	// Old-style RAR sets: a plain .rar that is an archive, with the old volumes of its name (RF-03).
	for i, f := range files {
		base, ok := PlainRARBase(f.Name)
		if !ok || !f.Archive || claimed[i] {
			continue
		}
		members := []int{i}
		for j, g := range files {
			if j == i || claimed[j] {
				continue
			}
			if other, old := LegacyVolumeBase(g.Name); old && other == base {
				members = append(members, j)
			}
		}
		for _, m := range members {
			claimed[m] = true
		}
		if len(members) > 1 {
			slices.SortFunc(members[1:], func(a, b int) int { return strings.Compare(files[a].Name, files[b].Name) })
		}
		if settled(files, members) {
			out = append(out, SambaUnit{Entry: entry, Files: pick(files, members)})
		}
	}

	// Single files: an archive by its bytes, or an old volume by its name (RF-03).
	for i, f := range files {
		if claimed[i] {
			continue
		}
		if _, old := LegacyVolumeBase(f.Name); !f.Archive && !old {
			continue
		}
		claimed[i] = true
		if settled(files, []int{i}) {
			out = append(out, SambaUnit{Entry: entry, Files: pick(files, []int{i})})
		}
	}
	return out
}

// sortedByVolume returns the indexes of the volumes in volume order (the first volume first).
func sortedByVolume(files []SambaFile, idx []int) []int {
	members := slices.Clone(idx)
	slices.SortStableFunc(members, func(a, b int) int {
		va, _ := ParseVolume(files[a].Name)
		vb, _ := ParseVolume(files[b].Name)
		return va.Index - vb.Index
	})
	return members
}

// isArchiveVolume reports whether the first volume of a set is an archive by its bytes: a set is taken
// from its first volume, and a file that is not an archive is not a set of volumes of one (RF-03).
func isArchiveVolume(files []SambaFile, members []int) bool {
	for _, m := range members {
		if v, _ := ParseVolume(files[m].Name); v.Index == 1 {
			return files[m].Archive
		}
	}
	return false
}

// settled reports whether every file of the unit is taken now: not copying, and not empty.
func settled(files []SambaFile, members []int) bool {
	for _, m := range members {
		if files[m].Copying || files[m].Size <= 0 {
			return false
		}
	}
	return true
}

// pick returns the files at the indexes, in their order.
func pick(files []SambaFile, idx []int) []SambaFile {
	out := make([]SambaFile, 0, len(idx))
	for _, i := range idx {
		out = append(out, files[i])
	}
	return out
}

// SourcePath is the file's place in _unassigned/, folders included, with "/" separators: the entry's
// name, then the place inside it (RF-27a). A loose file is its own entry.
func (f SambaFile) SourcePath() string {
	if f.Rel == "" {
		return f.EntryName
	}
	return path.Join(f.EntryName, f.Rel)
}
