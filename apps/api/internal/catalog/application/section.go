package application

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Errors of the section that the interfaces answer with their own status (RF-26, RF-27).
var (
	// ErrCopying means a file of the entry is still being copied, so the entry admits no action (RF-27).
	ErrCopying = errors.New("a file of the entry is still being copied")
	// ErrFileNotFound means the entry has no file at that path.
	ErrFileNotFound = errors.New("the entry has no file at that path")
	// ErrInvalidPath means a path is not a relative path inside an entry.
	ErrInvalidPath = errors.New("the path is not inside the entry")
)

// SectionDeps are what No asignados needs (RF-27): the registry, the library's files, the engine that
// changes them, and the extensions that tell a video and a subtitle.
type SectionDeps struct {
	Unassigned UnassignedRepository
	Files      Files
	Engine     *Engine
	// VideoExts and SubtitleExts are the extensions with their dot (VIDEO_EXTENSIONS, SUBTITLE_EXTENSIONS).
	VideoExts    []string
	SubtitleExts []string
	// Now may be nil (time.Now).
	Now func() time.Time
}

// Section reads No asignados as the disk holds it now (RF-27) and changes its entries through the
// engine. It keeps no state of its own: the readings of its files are in the registry.
type Section struct {
	d        SectionDeps
	videoExt map[string]bool
	subExt   map[string]bool
	now      func() time.Time
}

// NewSection builds the section over its ports.
func NewSection(d SectionDeps) *Section {
	s := &Section{d: d, now: d.Now}
	if s.now == nil {
		s.now = time.Now
	}
	s.videoExt = extSet(d.VideoExts)
	s.subExt = extSet(d.SubtitleExts)
	return s
}

// SectionEntry is an entry of No asignados as the section lists it (RF-27).
type SectionEntry struct {
	ID       int64
	Name     string
	Origin   string
	Relation string
	// Loose is a single file kept as it is: its only file is the entry itself.
	Loose bool
	// Status is domain.StatusCopying, StatusAssigning or StatusReady.
	Status string
	// Size is the total size of its files.
	Size  int64
	Files []SectionFile
}

// SectionFile is a file of an entry. Path is its place inside the entry, "/" separated; it is empty for
// a loose entry, whose file is the entry itself.
type SectionFile struct {
	Path    string
	Size    int64
	Copying bool
}

// sectionEntry is an entry as one reading of the library holds it: its record, with its files as the
// disk holds them now, and what is inside it, junk included.
type sectionEntry struct {
	domain.UnassignedEntry
	items   []domain.Item
	copying map[string]bool
}

// isCopying reports whether a file of the entry is still being copied.
func (e *sectionEntry) isCopying() bool {
	for _, c := range e.copying {
		if c {
			return true
		}
	}
	return false
}

// view returns the entry as the section lists it at now.
func (e *sectionEntry) view(now time.Time) SectionEntry {
	v := SectionEntry{
		ID: e.ID, Name: e.Name, Origin: e.Origin, Relation: e.Relation, Loose: e.Loose,
		Status: domain.EntryStatus(e.isCopying(), e.Lock.Held(now)),
	}
	files := slices.Clone(e.Files)
	slices.SortFunc(files, func(a, b domain.UnassignedFile) int { return strings.Compare(a.Rel, b.Rel) })
	for _, f := range files {
		v.Files = append(v.Files, SectionFile{Path: f.Rel, Size: f.Size, Copying: e.copying[f.Rel]})
		v.Size += f.Size
	}
	return v
}

// List reads No asignados now (RF-27): the folders and loose files of _unassigned/ that are not junk,
// each with its files and their state. The registry is brought in line with the disk first, so that
// what Samba copied shows up without waiting for the scan. Entries sort by name.
func (s *Section) List(ctx context.Context) ([]SectionEntry, error) {
	var out []SectionEntry
	err := s.d.Engine.Locked(func() error {
		entries, err := s.read(ctx)
		if err != nil {
			return err
		}
		now := s.now()
		for _, e := range entries {
			out = append(out, e.view(now))
		}
		return nil
	})
	return out, err
}

// TrashEntry sends a whole entry to the trash (RF-27, RF-30): each of its files goes with its own trash
// record, and its junk goes without the trash, with the folders it leaves.
func (s *Section) TrashEntry(ctx context.Context, id int64) error {
	return s.leaveEntry(ctx, id, leaveTrash)
}

// TrashFile sends one file of an entry to the trash (RF-27). A video goes with the subtitles of its name.
func (s *Section) TrashFile(ctx context.Context, id int64, rel string) error {
	return s.leaveFile(ctx, id, rel, leaveTrash)
}

// DeleteEntry deletes a whole entry for good (RF-27, RF-30): its files and its junk, without the trash.
func (s *Section) DeleteEntry(ctx context.Context, id int64) error {
	return s.leaveEntry(ctx, id, leaveDelete)
}

// DeleteFile deletes one file of an entry for good (RF-27). A video goes with the subtitles of its name.
func (s *Section) DeleteFile(ctx context.Context, id int64, rel string) error {
	return s.leaveFile(ctx, id, rel, leaveDelete)
}

// leaveMode says what happens to the files that leave an entry.
type leaveMode int

const (
	leaveTrash leaveMode = iota
	leaveDelete
)

// leaveEntry performs a change that takes every file of an entry out of it.
func (s *Section) leaveEntry(ctx context.Context, id int64, mode leaveMode) error {
	return s.leave(ctx, id, mode, func(e *sectionEntry) ([]domain.UnassignedFile, error) {
		return slices.Clone(e.Files), nil
	})
}

// leaveFile performs a change that takes one file out of an entry, and the subtitles of a video.
func (s *Section) leaveFile(ctx context.Context, id int64, rel string, mode leaveMode) error {
	if !domain.SafeEntryPath(rel) {
		return ErrInvalidPath
	}
	return s.leave(ctx, id, mode, func(e *sectionEntry) ([]domain.UnassignedFile, error) {
		if e.Loose {
			return nil, ErrFileNotFound
		}
		var chosen []domain.UnassignedFile
		found := false
		for _, f := range e.Files {
			if f.Rel == rel {
				found = true
				chosen = append(chosen, f)
			}
		}
		if !found {
			return nil, ErrFileNotFound
		}
		if !s.isVideo(rel) {
			return chosen, nil
		}
		for _, f := range e.Files {
			if !s.isSubtitle(f.Rel) || f.Rel == rel {
				continue
			}
			if _, named := domain.SubtitleSuffix(rel, f.Rel); named && path.Dir(f.Rel) == path.Dir(rel) {
				chosen = append(chosen, f)
			}
		}
		return chosen, nil
	})
}

// leave is the change of an entry's files: it refuses an entry that an assignment holds (RF-26) or
// that is still being copied (RF-27), then plans what leaves the entry, and performs it as one change.
func (s *Section) leave(ctx context.Context, id int64, mode leaveMode, choose func(*sectionEntry) ([]domain.UnassignedFile, error)) error {
	kind := "section.trash"
	if mode == leaveDelete {
		kind = "section.delete"
	}
	return s.d.Engine.Do(ctx, kind, "section:"+strconv.FormatInt(id, 10), func(b *Builder) error {
		entries, err := s.read(ctx)
		if err != nil {
			return err
		}
		e := findEntry(entries, id)
		if e == nil {
			return ErrEntryNotFound
		}
		if e.Lock.Held(s.now()) {
			return ErrAssigning
		}
		if e.isCopying() {
			return ErrCopying
		}
		chosen, err := choose(e)
		if err != nil {
			return err
		}
		return s.plan(ctx, b, e, chosen, mode)
	})
}

// plan adds to the change the moves of the chosen files and the folders that they leave empty. A file
// that is trashed gets its own trash record; a deleted one goes to the scratch folder, which the commit
// removes. When the entry keeps nothing, its junk goes too, and so do its folders (RF-27).
func (s *Section) plan(ctx context.Context, b *Builder, e *sectionEntry, chosen []domain.UnassignedFile, mode leaveMode) error {
	now := s.now()
	removed := map[string]bool{}
	var sources []string
	for i, f := range chosen {
		src := s.pathOf(e, f.Rel)
		switch mode {
		case leaveTrash:
			dir := randomName()
			entry, err := trashRecordOf(e, f, dir, now)
			if err != nil {
				return err
			}
			if err := b.Mkdir(s.d.Files.TrashPath(dir)); err != nil {
				return fmt.Errorf("catalog: create trash folder: %w", err)
			}
			if err := b.Move(src, s.d.Files.TrashPath(dir, entry.Files[0].Name)); err != nil {
				return fmt.Errorf("catalog: move to trash: %w", err)
			}
			b.Record(func(rec Records) error {
				if _, err := rec.InsertTrash(ctx, entry); err != nil {
					return fmt.Errorf("catalog: record trash entry: %w", err)
				}
				return nil
			})
		case leaveDelete:
			dst, err := b.scratchPath("removed", strconv.Itoa(i))
			if err != nil {
				return err
			}
			if err := b.Move(src, dst); err != nil {
				return fmt.Errorf("catalog: delete: %w", err)
			}
		}
		removed[f.Rel] = true
		sources = append(sources, f.Source)
	}
	if len(sources) > 0 {
		entryID := e.ID
		b.Record(func(rec Records) error { return rec.RemoveUnassignedFiles(ctx, entryID, sources) })
	}

	gone := true
	for _, f := range e.Files {
		if !removed[f.Rel] {
			gone = false
			break
		}
	}
	if !e.Loose {
		if err := s.leaveFolders(b, e.Name, e.items, removed, gone); err != nil {
			return err
		}
	}
	if gone {
		entryID := e.ID
		b.Record(func(rec Records) error { return rec.DeleteUnassignedEntry(ctx, entryID) })
	}
	return nil
}

// leaveFolders plans the junk and the folders of the folder name, an entry of No asignados, once the
// files in removed have left it (RF-27). items is what is inside it now. See domain.Vacate for which
// ones go: junk is deleted without the trash only when the entry is left empty (empty).
func (s *Section) leaveFolders(b *Builder, name string, items []domain.Item, removed map[string]bool, empty bool) error {
	junk, folders := domain.Vacate(items, removed, empty)
	for i, rel := range junk {
		dst, err := b.scratchPath("junk", strconv.Itoa(i))
		if err != nil {
			return err
		}
		if err := b.Move(s.folderPath(name, rel), dst); err != nil {
			return fmt.Errorf("catalog: remove junk: %w", err)
		}
	}
	for _, d := range folders {
		b.RemoveDir(s.folderPath(name, d))
	}
	if empty {
		b.RemoveDir(s.folderPath(name, ""))
	}
	return nil
}

// pathOf is the absolute path of a file or folder of an entry; "" is the entry itself.
func (s *Section) pathOf(e *sectionEntry, rel string) string {
	if e.Loose {
		return s.d.Files.UnassignedPath(e.Name)
	}
	return s.folderPath(e.Name, rel)
}

// folderPath is the absolute path of a file or folder inside the entry name of _unassigned/; "" is the
// entry itself.
func (s *Section) folderPath(name, rel string) string {
	if rel == "" {
		return s.d.Files.UnassignedPath(name)
	}
	return s.d.Files.UnassignedPath(name, filepath.FromSlash(rel))
}

// isVideo and isSubtitle tell the files of an entry by their extension.
func (s *Section) isVideo(rel string) bool {
	return s.videoExt[strings.ToLower(path.Ext(rel))]
}

func (s *Section) isSubtitle(rel string) bool {
	return s.subExt[strings.ToLower(path.Ext(rel))]
}

// trashRecordOf returns the trash record of a file that leaves an entry: where it was in its drop (RF-30)
// and what it came from. A Samba file has no batch.
func trashRecordOf(e *sectionEntry, f domain.UnassignedFile, dir string, now time.Time) (domain.TrashEntry, error) {
	origin := domain.TrashOriginUpload
	batch := e.BatchID
	if e.Origin == domain.OriginSamba {
		origin = domain.TrashOriginSamba
		batch = ""
	}
	name, rel := path.Base(f.Rel), path.Join(e.Name, f.Rel)
	if e.Loose {
		name, rel = e.Name, e.Name
	}
	return domain.NewTrashEntry(origin, domain.TrashReasonRemoved, dir,
		domain.Provenance{BatchID: batch, RelativePath: rel}, domain.TrashFile{Name: name, Size: f.Size}, now)
}

// read reads _unassigned/ now and brings the registry in line with it (RF-27). An entry that is gone from
// the disk leaves the registry, and a folder or loose file that is not in it joins as a Samba entry. Each
// file of an entry takes its size and modification time, and is copying when domain.Copying says so
// against the reading before. It must run while no change runs (Engine.Locked, or inside Engine.Do).
func (s *Section) read(ctx context.Context) ([]*sectionEntry, error) {
	now := s.now()
	tree, err := s.d.Files.Tree(s.d.Files.UnassignedPath())
	if err != nil {
		return nil, fmt.Errorf("catalog: read %s: %w", domain.UnassignedDir, err)
	}
	folders := domain.Entries(tree)
	onDisk := map[string]domain.Folder{}
	for _, f := range folders {
		onDisk[strings.ToLower(f.Name)] = f
	}
	rows, err := s.d.Unassigned.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []*sectionEntry
	err = s.d.Unassigned.Apply(ctx, func(rec Records) error {
		out = out[:0]
		seen := map[string]bool{}
		for _, row := range rows {
			key := strings.ToLower(row.Name)
			f, ok := onDisk[key]
			if !ok || f.Loose != row.Loose {
				if err := forget(ctx, rec, row); err != nil {
					return err
				}
				continue
			}
			seen[key] = true
			e, err := syncEntry(ctx, rec, row, f, now)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		for _, f := range folders {
			if seen[strings.ToLower(f.Name)] {
				continue
			}
			entry, err := domain.NewSambaEntry(f.Name, f.Loose, now)
			if err != nil {
				return err
			}
			e := &sectionEntry{items: f.Items, copying: map[string]bool{}}
			for _, it := range domain.Files(f.Items) {
				file := domain.UnassignedFile{Source: placeOf(f.Name, f.Loose, it.Rel), Rel: it.Rel, Size: it.Size, Mtime: it.ModTime.UnixMilli()}
				entry.Files = append(entry.Files, file)
				e.copying[it.Rel] = domain.Copying(nil, stampOf(it), now)
			}
			id, err := rec.InsertUnassigned(ctx, entry)
			if err != nil {
				return fmt.Errorf("catalog: record Samba entry: %w", err)
			}
			entry.ID = id
			e.UnassignedEntry = entry
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b *sectionEntry) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// syncEntry makes the files of a recorded entry match the disk (RF-27): new files are added, files that
// are gone leave, and the ones kept take the size and time of this reading.
func syncEntry(ctx context.Context, rec Records, row domain.UnassignedEntry, f domain.Folder, now time.Time) (*sectionEntry, error) {
	e := &sectionEntry{UnassignedEntry: row, items: f.Items, copying: map[string]bool{}}
	e.Files = nil
	known := map[string]domain.UnassignedFile{}
	for _, file := range row.Files {
		known[strings.ToLower(file.Rel)] = file
	}
	for _, it := range domain.Files(f.Items) {
		cur := stampOf(it)
		key := strings.ToLower(it.Rel)
		if old, ok := known[key]; ok {
			delete(known, key)
			file := domain.UnassignedFile{Source: old.Source, Rel: it.Rel, Size: it.Size, Mtime: it.ModTime.UnixMilli()}
			if old.Rel != file.Rel || old.Size != file.Size || old.Mtime != file.Mtime {
				if err := rec.UpdateUnassignedFile(ctx, row.ID, file); err != nil {
					return nil, err
				}
			}
			e.Files = append(e.Files, file)
			e.copying[it.Rel] = domain.CopyingKnown(row.Origin, previousStamp(old), cur, now)
			continue
		}
		file := domain.UnassignedFile{Source: placeOf(row.Name, row.Loose, it.Rel), Rel: it.Rel, Size: it.Size, Mtime: it.ModTime.UnixMilli()}
		if err := rec.AddUnassignedFiles(ctx, row.ID, []domain.UnassignedFile{file}); err != nil {
			return nil, fmt.Errorf("catalog: record file of %s: %w", row.Name, err)
		}
		e.Files = append(e.Files, file)
		e.copying[it.Rel] = domain.Copying(nil, cur, now)
	}
	var gone []string
	for _, old := range known {
		gone = append(gone, old.Source)
	}
	if len(gone) > 0 {
		slices.Sort(gone)
		if err := rec.RemoveUnassignedFiles(ctx, row.ID, gone); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// forget removes an entry that is gone from the disk, with its files.
func forget(ctx context.Context, rec Records, row domain.UnassignedEntry) error {
	var sources []string
	for _, f := range row.Files {
		sources = append(sources, f.Source)
	}
	if len(sources) > 0 {
		if err := rec.RemoveUnassignedFiles(ctx, row.ID, sources); err != nil {
			return err
		}
	}
	if err := rec.DeleteUnassignedEntry(ctx, row.ID); err != nil {
		return fmt.Errorf("catalog: forget %s: %w", row.Name, err)
	}
	return nil
}

// placeOf is where a file of an entry lives in _unassigned/, which is the source of a file that
// Samba copied there: it was never moved by the app.
func placeOf(name string, loose bool, rel string) string {
	if loose {
		return path.Join(domain.UnassignedDir, name)
	}
	return path.Join(domain.UnassignedDir, name, rel)
}

// stampOf is the size and modification time of an item as a reading keeps them.
func stampOf(it domain.Item) domain.Stamp {
	return domain.Stamp{Size: it.Size, ModTime: it.ModTime}
}

// previousStamp is the reading a file had in the registry, or nil when it was never read.
func previousStamp(f domain.UnassignedFile) *domain.Stamp {
	if f.Mtime == 0 {
		return nil
	}
	return &domain.Stamp{Size: f.Size, ModTime: time.UnixMilli(f.Mtime)}
}

// findEntry returns the entry with the id, or nil.
func findEntry(entries []*sectionEntry, id int64) *sectionEntry {
	for _, e := range entries {
		if e.ID == id {
			return e
		}
	}
	return nil
}
