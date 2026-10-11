package domain

import (
	"path"
	"strings"
)

// UnitContent is what the filter kept of one work unit (RF-07): a file uploaded on its
// own, or the content of an archive or of a set of volumes.
type UnitContent struct {
	// Leader is the id the unit is stored under: its job, or the group of its set.
	Leader JobID
	// Jobs are the jobs of the unit: one, or the volumes of a set in order.
	Jobs []*UploadJob
	// Extracted says the content is in the extraction folder (an archive or a set), and not a
	// file uploaded on its own.
	Extracted bool
	// Kept is what the filter recorded for the unit. Discarded files are not part of it.
	Kept []FilterEntry
}

// EntryPlan is an entry of No asignados as a batch decides it (RF-08, RF-08a): the files it
// takes, each with its place in the entry. The catalog sanitizes Name and gives the entry
// its final name.
type EntryPlan struct {
	// Key is what the move is for, so that a repeated move finds the same entry: the batch
	// when the batch has a relation, or the leader of the unit otherwise.
	Key string
	// Name is the name before sanitizing: the batch's name, or the name of an archive or
	// of a file without its extension (RF-08). A loose file keeps its extension.
	Name string
	// Loose is a single video kept as it is, without a folder (RF-08, RF-08a).
	Loose bool
	Files []PlannedFile
}

// PlannedFile is one file of an entry: where it is in its unit, and where it goes in the entry.
type PlannedFile struct {
	// Unit is the leader of the unit the file belongs to.
	Unit JobID
	// Path is the file's place in the extraction folder of its unit, with "/" separators. It
	// is empty for a file uploaded on its own, which is staged under its job's id.
	Path string
	// Rel is its place inside the entry, with "/" separators; empty for a loose entry.
	Rel  string
	Size int64
	Kind FileKind
}

// PlanEntries decides the entries of the content a batch kept (RF-08, RF-08a). With
// relation none every unit is its own entry (RF-08), and a single video without subtitles
// is a loose file. With parts or episodes the whole batch is one entry, in a folder named
// after the batch (the folder dropped in the browser, RF-01a) or after its first archive or
// file. The folders inside a dropped folder keep their place in the entry, without the
// dropped folder itself. Units with nothing kept give no entry.
func PlanEntries(batch Batch, units []UnitContent) []EntryPlan {
	if batch.Relation == RelationNone {
		plans := make([]EntryPlan, 0, len(units))
		for _, u := range units {
			if p, ok := planUnit(u); ok {
				plans = append(plans, p)
			}
		}
		return plans
	}
	return planBatch(batch, units)
}

// planUnit is the entry of one unit when the batch has no relation.
func planUnit(u UnitContent) (EntryPlan, bool) {
	kept := keptOf(u)
	if len(kept) == 0 {
		return EntryPlan{}, false
	}
	first := u.Jobs[0]
	loose := !u.Extracted && len(kept) == 1 && kept[0].Kind == KindVideo
	plan := EntryPlan{Key: string(u.Leader), Name: EntryBase(first.FileName), Loose: loose}
	if loose {
		plan.Name = first.FileName
	}
	for _, k := range kept {
		f := PlannedFile{Unit: u.Leader, Size: k.Size, Kind: k.Kind}
		switch {
		case loose:
			// A loose entry is the file itself: it has no place of its own.
		case u.Extracted:
			f.Path, f.Rel = k.Path, k.Path
		default:
			f.Rel = path.Base(originOf(first))
		}
		plan.Files = append(plan.Files, f)
	}
	return plan, true
}

// planBatch is the one entry of a batch with a relation (RF-08a).
func planBatch(batch Batch, units []UnitContent) []EntryPlan {
	plan := EntryPlan{Key: string(batch.ID), Name: batch.Name}
	for _, u := range units {
		kept := keptOf(u)
		if len(kept) == 0 {
			continue
		}
		first := u.Jobs[0]
		if plan.Name == "" {
			plan.Name = EntryBase(first.FileName)
		}
		prefix := folderInside(originOf(first))
		for _, k := range kept {
			f := PlannedFile{Unit: u.Leader, Size: k.Size, Kind: k.Kind}
			if u.Extracted {
				f.Path = k.Path
				f.Rel = path.Join(prefix, k.Path)
			} else {
				f.Rel = path.Join(prefix, path.Base(originOf(first)))
			}
			plan.Files = append(plan.Files, f)
		}
	}
	if len(plan.Files) == 0 {
		return nil
	}
	return []EntryPlan{plan}
}

// keptOf returns what the filter kept of a unit: videos and subtitles.
func keptOf(u UnitContent) []FilterEntry {
	var kept []FilterEntry
	for _, k := range u.Kept {
		if k.Kind.Kept() {
			kept = append(kept, k)
		}
	}
	return kept
}

// originOf is the path a job's file had in the drop (RF-01a): its path in the folder dropped
// in the browser, or its name when it was dropped on its own.
func originOf(job *UploadJob) string {
	if job.RelativePath != "" {
		return job.RelativePath
	}
	return job.FileName
}

// folderInside returns the folders of origin below the dropped folder: the dropped folder
// is the entry itself, so its name goes and what lies in its subfolders stays. A file
// dropped on its own has none.
func folderInside(origin string) string {
	dir := path.Dir(origin)
	if !strings.Contains(origin, "/") || dir == "." {
		return ""
	}
	if i := strings.Index(dir, "/"); i >= 0 {
		return dir[i+1:]
	}
	return ""
}

// EntryBase returns the name an entry takes from the file it came with, without its
// extension (RF-08): the base name of the set for a volume of a multi-volume archive
// (movie.7z.001 and movie.part2.rar give movie), the name of an archive or of a file
// otherwise.
func EntryBase(fileName string) string {
	if m := partStyle.FindStringSubmatch(fileName); m != nil {
		return m[1]
	}
	if m := numberedStyle.FindStringSubmatch(fileName); m != nil {
		return stripExt(m[1])
	}
	return stripExt(fileName)
}

// stripExt removes the extension of a file name, when it has one.
func stripExt(name string) string {
	ext := path.Ext(name)
	if len(ext) <= 1 {
		return name
	}
	return strings.TrimSuffix(name, ext)
}
