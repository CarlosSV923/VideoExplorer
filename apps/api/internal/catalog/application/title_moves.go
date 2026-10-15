package application

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Moving and trashing parts of a title (spec RF-44): the title as a whole, one season, or one file with its
// subtitles. Both are one change of the library each (RNF-04): they go through the engine, and the registry
// forgets or keeps the files as the change says.

// ScopeKind says what a move or a trash takes from a title.
type ScopeKind string

// The scopes of a move or a trash (RF-44).
const (
	ScopeTitle  ScopeKind = "title"
	ScopeSeason ScopeKind = "season"
	ScopeFile   ScopeKind = "file"
)

// Scope is what a move or a trash takes: the whole title, a season (its number), or one video with its subtitles
// (its registry id). A movie's part, or a version, is a file.
type Scope struct {
	Kind   ScopeKind
	Season int
	FileID int64
}

// ErrScopeNotFound means the scope names nothing of the title that is in the library (RF-44).
var ErrScopeNotFound = errors.New("nothing of the title is in that scope")

// unit is a video of the scope with the subtitles that go with it: they always move and go to the trash together.
type unit struct {
	video domain.LibraryFile
	subs  []domain.LibraryFile
}

// unitsOf returns the videos of the title that the scope takes, each with its live subtitles, in the order of the
// registry. A scope with nothing in it is ErrScopeNotFound.
func unitsOf(st editState, s Scope) ([]unit, error) {
	subs := map[string][]domain.LibraryFile{}
	for _, f := range st.files {
		if f.Kind == domain.FileSubtitle && isLive(f) {
			key := strings.ToLower(f.VideoRel)
			subs[key] = append(subs[key], f)
		}
	}
	var out []unit
	for _, f := range st.files {
		if f.Kind != domain.FileVideo || !isLive(f) {
			continue
		}
		switch s.Kind {
		case ScopeTitle:
		case ScopeSeason:
			if !f.Place.Episode || f.Place.Season != s.Season {
				continue
			}
		case ScopeFile:
			if f.ID != s.FileID {
				continue
			}
		default:
			return nil, fmt.Errorf("%w: scope %q", ErrScopeNotFound, s.Kind)
		}
		out = append(out, unit{video: f, subs: subs[strings.ToLower(f.Rel)]})
	}
	if len(out) == 0 {
		return nil, ErrScopeNotFound
	}
	return out, nil
}

// MoveToUnassigned moves what the scope takes to No asignados (RF-44): a title keeps its folder's name, a season
// is "<Título> (<Año>) Season NN", and a video with subtitles keeps its name without the extension as its folder; a
// video without subtitles is a loose file (RF-08). Their origin is samba and their relation none, as an import moves
// them (RF-29). The registry forgets them, and the title too when nothing of it is left.
func (e *Edits) MoveToUnassigned(ctx context.Context, titleID int64, s Scope) error {
	return e.d.Engine.Do(ctx, "unassign", fmt.Sprintf("title:%d", titleID), func(b *Builder) error {
		st, err := e.load(ctx, titleID)
		if err != nil {
			return err
		}
		units, err := unitsOf(st, s)
		if err != nil {
			return err
		}
		entry, files, err := e.entryOf(st, s, units)
		if err != nil {
			return err
		}
		// The section folder may not exist yet on a library that was never scanned into it (RNF-04: a move needs its folder).
		if err := b.Mkdir(b.files.UnassignedPath()); err != nil {
			return err
		}
		if !entry.Loose {
			if err := b.Mkdir(b.files.UnassignedPath(entry.Name)); err != nil {
				return err
			}
		}
		froms := make([]string, 0, len(files))
		for _, f := range files {
			to := b.files.UnassignedPath(entry.Name, f.Rel)
			if entry.Loose {
				to = b.files.UnassignedPath(entry.Name)
			}
			if f.Rel != "" {
				if err := b.Mkdir(path.Dir(to)); err != nil {
					return err
				}
			}
			if err := b.Move(b.files.LibraryPath(f.Source), to); err != nil {
				return err
			}
			froms = append(froms, f.Source)
		}
		if err := e.removeEmptiedFolders(b, froms, nil); err != nil {
			return err
		}
		moved := map[int64]bool{}
		for _, u := range units {
			moved[u.video.ID] = true
			for _, s := range u.subs {
				moved[s.ID] = true
			}
		}
		left := 0
		for _, f := range st.files {
			if !moved[f.ID] {
				left++
			}
		}
		b.Record(func(rec Records) error {
			id, err := rec.InsertUnassigned(ctx, entry)
			if err != nil {
				return err
			}
			if err := rec.AddUnassignedFiles(ctx, id, files); err != nil {
				return err
			}
			for _, u := range units {
				for _, sub := range u.subs {
					if err := rec.DeleteLibraryFile(ctx, sub.ID); err != nil {
						return err
					}
				}
				if err := rec.DeleteLibraryFile(ctx, u.video.ID); err != nil {
					return err
				}
			}
			if left == 0 {
				return rec.DeleteLibraryTitle(ctx, st.title.ID)
			}
			return nil
		})
		return nil
	})
}

// entryOf returns the entry of No asignados that the scope becomes and the files it holds, with their paths
// inside the entry (RF-44). The name is free in _unassigned/ (RF-08: " (2)", " (3)"…).
func (e *Edits) entryOf(st editState, s Scope, units []unit) (domain.UnassignedEntry, []domain.UnassignedFile, error) {
	names, err := e.d.Files.Names(e.d.Files.UnassignedPath())
	if err != nil {
		return domain.UnassignedEntry{}, nil, err
	}
	taken := domain.NewNameSet(names...)
	var (
		name  string
		loose bool
		place func(rel string) string
	)
	switch s.Kind {
	case ScopeTitle:
		name = domain.TitleEntryName(st.title.Folder)
		place = func(rel string) string { return strings.TrimPrefix(rel, st.title.Folder+"/") }
	case ScopeSeason:
		name, err = domain.SeasonEntryName(st.title, s.Season)
		if err != nil {
			return domain.UnassignedEntry{}, nil, err
		}
		place = path.Base
	case ScopeFile:
		u := units[0]
		if len(u.subs) == 0 {
			loose = true
			name = path.Base(u.video.Rel)
			place = func(string) string { return "" }
		} else {
			name = domain.SubtitledEntryName(u.video.Rel)
			place = path.Base
		}
	}
	name = domain.FreeName(name, loose, taken)
	entry, err := domain.NewImportedEntry("library:"+randomName(), name, loose, e.now())
	if err != nil {
		return domain.UnassignedEntry{}, nil, err
	}
	var files []domain.UnassignedFile
	for _, u := range units {
		for _, f := range append([]domain.LibraryFile{u.video}, u.subs...) {
			rel := place(f.Rel)
			uf, err := domain.NewUnassignedFile(f.Rel, rel, f.Size, loose)
			if err != nil {
				return domain.UnassignedEntry{}, nil, err
			}
			files = append(files, uf)
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Source < files[j].Source })
	return entry, files, nil
}

// Trash sends what the scope takes to the trash, with its subtitles (RF-44, RF-30): one trash entry per video, with
// origin library and reason removed, so that RF-31 restores it to its place. The registry keeps the rows, marked as
// trashed, and a title whose files are all in the trash leaves the library (RF-30).
func (e *Edits) Trash(ctx context.Context, titleID int64, s Scope) error {
	return e.d.Engine.Do(ctx, "trash", fmt.Sprintf("title:%d", titleID), func(b *Builder) error {
		st, err := e.load(ctx, titleID)
		if err != nil {
			return err
		}
		units, err := unitsOf(st, s)
		if err != nil {
			return err
		}
		now := e.now()
		var froms []string
		type trashing struct {
			entry domain.TrashEntry
			ids   []int64
		}
		var entries []trashing
		for _, u := range units {
			dir := randomName()
			all := append([]domain.LibraryFile{u.video}, u.subs...)
			files := make([]domain.TrashFile, 0, len(all))
			var ids []int64
			for _, f := range all {
				files = append(files, domain.TrashFile{Name: path.Base(f.Rel), Size: f.Size})
				ids = append(ids, f.ID)
			}
			entry, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonRemoved, dir,
				domain.Provenance{RelativePath: u.video.Rel}, files, now)
			if err != nil {
				return err
			}
			if err := b.Mkdir(b.files.TrashPath(dir)); err != nil {
				return err
			}
			for i, f := range all {
				if err := b.Move(b.files.LibraryPath(f.Rel), b.files.TrashPath(dir, files[i].Name)); err != nil {
					return fmt.Errorf("move to trash: %w", err)
				}
				froms = append(froms, f.Rel)
			}
			entries = append(entries, trashing{entry: entry, ids: ids})
		}
		if err := e.removeEmptiedFolders(b, froms, nil); err != nil {
			return err
		}
		b.Record(func(rec Records) error {
			for _, t := range entries {
				id, err := rec.InsertTrash(ctx, t.entry)
				if err != nil {
					return err
				}
				if err := rec.TrashLibraryFiles(ctx, t.ids, id); err != nil {
					return err
				}
			}
			return nil
		})
		return nil
	})
}
