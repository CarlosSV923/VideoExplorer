package application

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Commit performs an edit of a title as one change of the library (RNF-04, RF-44): the request is checked
// again under the engine's lock, its revision must be the one the plan was made from (plan_changed, RF-25), and
// the files move in two passes, so that a file never takes the place of another that has not moved yet. The
// trash, the renamed versions and the registry change in the same commit; a failure undoes all of it.
func (e *Edits) Commit(ctx context.Context, titleID int64, req EditRequest) (EditResult, error) {
	var res EditResult
	err := e.d.Engine.Do(ctx, "edit", fmt.Sprintf("title:%d", titleID), func(b *Builder) error {
		st, err := e.load(ctx, titleID)
		if err != nil {
			return err
		}
		if st.revision != req.Revision {
			return ErrPlanChanged
		}
		ev, err := e.assess(ctx, st, req)
		if err != nil {
			return err
		}
		if !ev.plan.Valid {
			return &EditInvalidError{Plan: ev.plan}
		}
		return e.record(ctx, b, ev, &res)
	})
	if err != nil {
		return EditResult{}, err
	}
	return res, nil
}

// record adds the steps and the registry records of a valid evaluation (RNF-04). It runs under the engine's lock.
// Nothing is added when the edit changes nothing.
func (e *Edits) record(ctx context.Context, b *Builder, ev *editEval, res *EditResult) error {
	st, id := ev.st, ev.id
	targetID := st.title.ID
	if id.merge != nil {
		targetID = id.merge.ID
	}
	res.TitleID, res.Folder, res.Merged = targetID, id.folder, id.merge != nil

	title := identityOf(st.title, id)
	identityChanged := title.Kind != st.title.Kind || title.Title != st.title.Title || title.DiscTitle != st.title.DiscTitle ||
		title.Year != st.title.Year || title.TMDBID != st.title.TMDBID || title.IDTag != st.title.IDTag || title.Folder != st.title.Folder
	rows := e.changedRows(ev, targetID)
	if len(ev.moves) == 0 && len(ev.trash) == 0 && len(ev.renames) == 0 && len(rows.values) == 0 && !identityChanged && id.merge == nil {
		return nil
	}
	res.Changed = true

	if len(ev.moves) > 0 || len(ev.renames) > 0 || len(ev.trash) > 0 {
		if err := b.Mkdir(b.files.OpsPath(b.opID)); err != nil {
			return err
		}
	}
	// The files that move leave their places first: a file takes a place only once it is free (RF-44).
	scratch := map[string]string{}
	for i, m := range ev.moves {
		s := b.files.OpsPath(b.opID, fmt.Sprintf("move-%d", i))
		if err := b.Move(b.files.LibraryPath(m.From), s); err != nil {
			return err
		}
		scratch[strings.ToLower(m.From)] = s
	}
	trashed, err := e.trashReplaced(b, ev)
	if err != nil {
		return err
	}
	for _, m := range ev.renames {
		if err := b.Move(b.files.LibraryPath(m.From), b.files.LibraryPath(m.To)); err != nil {
			return err
		}
	}
	for _, m := range ev.moves {
		if err := b.Mkdir(b.files.LibraryPath(path.Dir(m.To))); err != nil {
			return err
		}
		if err := b.Move(scratch[strings.ToLower(m.From)], b.files.LibraryPath(m.To)); err != nil {
			return err
		}
	}
	if err := e.removeEmptied(b, ev); err != nil {
		return err
	}

	b.Record(func(rec Records) error {
		for _, rowID := range rows.order {
			if rows.released[rowID] {
				if err := rec.ReleaseLibraryFileRel(ctx, rowID); err != nil {
					return err
				}
			}
		}
		if trashed != nil {
			entryID, err := rec.InsertTrash(ctx, trashed.entry)
			if err != nil {
				return err
			}
			if err := rec.TrashLibraryFiles(ctx, trashed.ids, entryID); err != nil {
				return err
			}
		}
		for _, rowID := range rows.order {
			if err := rec.SaveLibraryFile(ctx, rows.values[rowID]); err != nil {
				return err
			}
		}
		if id.merge != nil {
			return rec.DeleteLibraryTitle(ctx, st.title.ID)
		}
		if identityChanged {
			return rec.SaveLibraryTitle(ctx, title)
		}
		return nil
	})
	return nil
}

// identityOf returns the registry title the edit leaves a title as, with its folder and its tag (RF-44, RF-45).
// A merge into another title is not an identity of this one: the caller deletes this title's row instead.
func identityOf(t domain.LibraryTitle, id *editTitle) domain.LibraryTitle {
	out := t
	out.Kind, out.Title, out.DiscTitle, out.Year = id.kind, id.display, id.title.Name, id.title.Year
	out.TMDBID, out.IDTag, out.Folder = id.tmdbID, id.title.IDTag, id.folder
	if id.tmdbID == 0 {
		out.IDTag = false
	}
	return out
}

// changedRowSet is what the registry changes of an edit: the new values of each row, and which rows change their
// paths (they are released first, so that the registry's unique paths hold whatever the order).
type changedRowSet struct {
	values   map[int64]domain.LibraryFile
	order    []int64
	released map[int64]bool
}

// changedRows returns the registry rows an edit changes: the videos and subtitles it places, the old versions it
// renames, and, when the title joins another, every row of the title (the trashed ones too: RF-31).
func (e *Edits) changedRows(ev *editEval, targetID int64) changedRowSet {
	out := changedRowSet{values: map[int64]domain.LibraryFile{}, released: map[int64]bool{}}
	set := func(old, next domain.LibraryFile) {
		if old.Rel != next.Rel {
			out.released[old.ID] = true
		}
		if _, seen := out.values[old.ID]; !seen {
			out.order = append(out.order, old.ID)
		}
		out.values[old.ID] = next
	}
	for _, v := range ev.videos {
		if v.bad || !v.chosen || v.skipped {
			continue
		}
		next := v.file
		next.TitleID, next.Rel, next.Place, next.Version = targetID, v.finalOfVideo(), v.place, v.version
		if !sameRow(v.file, next) {
			set(v.file, next)
		}
	}
	for _, s := range ev.subs {
		if s.bad || !s.chosen || s.video == nil || s.video.bad || !s.video.chosen || s.video.skipped {
			continue
		}
		next := s.file
		next.TitleID, next.Rel = targetID, finalPath(s.video.moves, s.file.Rel)
		next.Language, next.Forced, next.SDH = s.language, s.choice.Forced, s.choice.SDH
		if !sameRow(s.file, next) {
			set(s.file, next)
		}
	}
	for _, m := range ev.renames {
		row, ok := ev.rowAt(m.From)
		if !ok || row.ID == 0 {
			continue
		}
		next := row
		next.Rel = m.To
		if row.Kind == domain.FileVideo {
			next.Version = m.Version
		}
		if !sameRow(row, next) {
			set(row, next)
		}
	}
	if id := ev.id; id.merge != nil {
		for _, f := range ev.st.files {
			if _, done := out.values[f.ID]; done {
				continue
			}
			next := f
			next.TitleID = targetID
			set(f, next)
		}
	}
	sort.Slice(out.order, func(i, j int) bool { return out.order[i] < out.order[j] })
	return out
}

// sameRow reports whether two versions of a registry row hold the same values the edit changes.
func sameRow(a, b domain.LibraryFile) bool {
	return a.TitleID == b.TitleID && a.Rel == b.Rel && a.Place == b.Place && a.Version == b.Version &&
		a.Language == b.Language && a.Forced == b.Forced && a.SDH == b.SDH
}

// rowAt returns the registry row of the file at rel, when the title has one.
func (ev *editEval) rowAt(rel string) (domain.LibraryFile, bool) {
	for _, f := range ev.st.files {
		if strings.EqualFold(f.Rel, rel) && isLive(f) {
			return f, true
		}
	}
	return domain.LibraryFile{}, false
}

// trashed is the trash entry of the replaced files of an edit and the registry rows that go with it.
type trashed struct {
	entry domain.TrashEntry
	ids   []int64
}

// trashReplaced sends the files that a duplicate's answer Replace takes away to one trash entry (RF-24, RF-30).
// It returns nil when no file is replaced.
func (e *Edits) trashReplaced(b *Builder, ev *editEval) (*trashed, error) {
	if len(ev.trash) == 0 {
		return nil, nil
	}
	dir := randomName()
	files := make([]domain.TrashFile, 0, len(ev.trash))
	var ids []int64
	for _, m := range ev.trash {
		files = append(files, domain.TrashFile{Name: path.Base(m.From), Size: m.Size})
		if m.ID != 0 {
			ids = append(ids, m.ID)
		}
	}
	entry, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonReplaced, dir,
		domain.Provenance{RelativePath: ev.trash[0].From}, files, e.now())
	if err != nil {
		return nil, err
	}
	if err := b.Mkdir(b.files.TrashPath(dir)); err != nil {
		return nil, err
	}
	for i, m := range ev.trash {
		if err := b.Move(b.files.LibraryPath(m.From), b.files.TrashPath(dir, files[i].Name)); err != nil {
			return nil, fmt.Errorf("move to trash: %w", err)
		}
	}
	return &trashed{entry: entry, ids: ids}, nil
}

// removeEmptied removes the folders of the title that the moves leave empty (RF-44: "una carpeta que queda vacía
// se quita"). A folder that receives a file, or that holds anything the edit does not move, stays. Only the
// title's folder and its seasons are considered: the library's roots are never removed. The paths keep their
// letter case: they are the paths on disk.
func (e *Edits) removeEmptied(b *Builder, ev *editEval) error {
	movedOut := map[string]bool{}
	keep := map[string]bool{}
	var froms []string
	for _, m := range append(append([]domain.Move{}, ev.moves...), ev.renames...) {
		if !strings.EqualFold(m.From, m.To) {
			movedOut[strings.ToLower(m.From)] = true
			froms = append(froms, m.From)
		}
		for k := range foldersOf(m.To) {
			keep[k] = true
		}
	}
	for _, m := range ev.trash {
		movedOut[strings.ToLower(m.From)] = true
		froms = append(froms, m.From)
	}
	candidates := map[string]string{}
	for _, from := range froms {
		for k, d := range foldersOf(from) {
			if !keep[k] {
				candidates[k] = d
			}
		}
	}
	keys := make([]string, 0, len(candidates))
	for k := range candidates {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if depth(keys[i]) != depth(keys[j]) {
			return depth(keys[i]) > depth(keys[j])
		}
		return keys[i] < keys[j]
	})
	removed := map[string]bool{}
	for _, k := range keys {
		dir := candidates[k]
		exists, err := e.d.Files.Exists(e.d.Files.LibraryPath(dir))
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		names, err := e.d.Files.Names(e.d.Files.LibraryPath(dir))
		if err != nil {
			return err
		}
		empty := true
		for _, n := range names {
			child := strings.ToLower(dir + "/" + n)
			if !movedOut[child] && !removed[child] {
				empty = false
				break
			}
		}
		if empty {
			b.RemoveDir(e.d.Files.LibraryPath(dir))
			removed[k] = true
		}
	}
	return nil
}

// foldersOf returns the folders of a library path that the edit may remove: the title's folder and its season
// folder (depth 2 and 3). The library's root is never one of them. The keys are in lower case; the values keep
// their letter case.
func foldersOf(rel string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(rel, "/")
	for k := 2; k < len(parts); k++ {
		d := strings.Join(parts[:k], "/")
		out[strings.ToLower(d)] = d
	}
	return out
}

// depth returns the number of components of a library folder path.
func depth(p string) int {
	return strings.Count(p, "/") + 1
}
