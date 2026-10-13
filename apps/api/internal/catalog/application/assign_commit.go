package application

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// PlacedFile is a video that the commit placed: its file in the entry and its library path (spec §5).
type PlacedFile struct {
	File string
	Path string
}

// Result is what a commit did (RF-25, RF-26): the videos placed, and whether the entry disappeared
// because nothing was left in it.
type Result struct {
	EntryID      int64
	EntryRemoved bool
	Placed       []PlacedFile
}

// Commit performs the request of the entry as one change of the library (RF-25): the registry gets the
// titles it did not have, the videos and their subtitles move to their final paths, the replaced files
// go to the trash and the versions take their labels, and the entry forgets what left it. The request
// is checked again under the engine's lock: a request that is not valid is refused with a
// PlanInvalidError and nothing changes. A failure undoes the whole change (RNF-04). What the request
// does not place stays in the entry, and the entry disappears when nothing is left in it and no video
// or subtitle is held in a junk folder (RF-26, RF-27).
func (a *Assignments) Commit(ctx context.Context, entryID int64, token string, req Request) (Result, error) {
	entry, err := a.entry(ctx, entryID)
	if err != nil {
		return Result{}, err
	}
	if err := a.requireLock(entry, token); err != nil {
		return Result{}, err
	}
	p, err := a.prepare(ctx, entry, req)
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = a.d.Engine.Do(ctx, "assign", entry.Source, func(b *Builder) error {
		// The entry is read from the disk first: a file that arrived since the form was opened is
		// part of the entry (RF-27), and one still being copied stops the commit.
		entries, err := a.section.read(ctx)
		if err != nil {
			return err
		}
		e := findEntry(entries, entryID)
		if e == nil {
			return ErrEntryNotFound
		}
		if e.isCopying() {
			return ErrCopying
		}
		fresh, ok, err := a.d.Unassigned.ByID(ctx, entryID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrEntryNotFound
		}
		if err := a.requireLock(fresh, token); err != nil {
			return err
		}
		if !sameFiles(entry, fresh) {
			// The entry changed since it was read: its choices are read again under the lock.
			if p, err = a.prepare(ctx, fresh, req); err != nil {
				return err
			}
		}
		ev, err := a.assess(ctx, p)
		if err != nil {
			return err
		}
		if !ev.plan.Valid {
			return &PlanInvalidError{Plan: ev.plan}
		}
		return a.record(ctx, b, p, ev, &result)
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// sameFiles reports whether two readings of an entry hold the same files.
func sameFiles(a, b domain.UnassignedEntry) bool {
	return a.ID == b.ID && slices.Equal(a.Files, b.Files)
}

// record adds the steps and the catalog records of an evaluation that is valid (RNF-04): one placement
// per place, performed in order, then the bookkeeping of the entry. It runs under the engine's lock.
func (a *Assignments) record(ctx context.Context, b *Builder, p *prepared, ev *evaluation, out *Result) error {
	now := a.now()
	entry := p.entry

	// The titles that a placement creates are inserted once, before the first record that needs them.
	titleIDs := map[string]*int64{}
	for _, g := range ev.places {
		if !g.perform || !g.target.newTitle {
			continue
		}
		key := strings.ToLower(g.target.folder)
		if titleIDs[key] != nil {
			continue
		}
		title, err := newTitleOf(g.target, now)
		if err != nil {
			return err
		}
		id := new(int64)
		titleIDs[key] = id
		b.Record(func(rec Records) error {
			v, err := rec.InsertLibraryTitle(ctx, title)
			if err != nil {
				return err
			}
			*id = v
			return nil
		})
	}
	titleID := func(t *target) int64 {
		if t.newTitle {
			return *titleIDs[strings.ToLower(t.folder)]
		}
		return t.libraryID
	}

	moved := map[string]bool{}
	for _, g := range ev.places {
		if !g.perform {
			continue
		}
		files, err := libraryFilesOf(g, p, now)
		if err != nil {
			return err
		}
		target := g.target
		if err := decisionBuild(ctx, g.plan, now, func(rec Records) error {
			return rec.InsertLibraryFiles(ctx, titleID(target), files)
		})(b); err != nil {
			return err
		}
		for _, vs := range g.videos {
			moved[vs.rel] = true
			out.Placed = append(out.Placed, PlacedFile{File: vs.rel, Path: finalOf(g.plan, vs.incoming.Source)})
		}
		for _, ss := range p.subtitles {
			if ss.video != nil && ss.video.group == g && ss.chosen && len(ss.issues) == 0 {
				moved[ss.rel] = true
			}
		}
	}

	if err := a.bookkeeping(ctx, b, entry, moved, out); err != nil {
		return err
	}
	return nil
}

// bookkeeping forgets the files that left the entry, removes the folders the moves emptied, and
// deletes the entry when nothing is left in it, or releases its lock when something is (RF-26).
func (a *Assignments) bookkeeping(ctx context.Context, b *Builder, entry domain.UnassignedEntry, moved map[string]bool, out *Result) error {
	var sources []string
	var remaining []domain.UnassignedFile
	for _, f := range entry.Files {
		if moved[f.Rel] {
			sources = append(sources, f.Source)
		} else {
			remaining = append(remaining, f)
		}
	}
	// What is inside the entry now, junk included. A video or subtitle inside a junk folder is held
	// (RF-27): the entry is not empty while it is there, and nothing of it is removed.
	var items []domain.Item
	held := false
	if !entry.Loose {
		var err error
		items, err = a.d.Files.Tree(a.d.Files.UnassignedPath(entry.Name))
		if err != nil {
			return fmt.Errorf("catalog: read entry %s: %w", entry.Name, err)
		}
		held = domain.Held(items, a.isMedia)
	}
	gone := len(remaining) == 0 && !held
	out.EntryID = entry.ID
	out.EntryRemoved = gone

	if !entry.Loose && len(sources) > 0 {
		// The folders the moves empty go, and the junk goes too when nothing is left in the entry (RF-27).
		if err := a.section.leaveFolders(b, entry.Name, items, moved, gone); err != nil {
			return err
		}
	}

	entryID := entry.ID
	b.Record(func(rec Records) error {
		if len(sources) > 0 {
			if err := rec.RemoveUnassignedFiles(ctx, entryID, sources); err != nil {
				return err
			}
		}
		if gone {
			return rec.DeleteUnassignedEntry(ctx, entryID)
		}
		return rec.ReleaseAssignLock(ctx, entryID)
	})
	return nil
}

// newTitleOf returns the registry title that a placement creates.
func newTitleOf(t *target, now time.Time) (domain.LibraryTitle, error) {
	name := t.display
	if name == "" {
		name = t.title.Name
	}
	return domain.NewLibraryTitle(t.kind, name, t.title.Name, t.title.Year, t.title.TMDBID, t.title.IDTag, t.folder, now)
}

// libraryFilesOf returns the registry rows of a placement, in order: each video, then its subtitles
// (a subtitle's video must be stored before it, RF-23).
func libraryFilesOf(g *placement, p *prepared, now time.Time) ([]domain.LibraryFile, error) {
	var files []domain.LibraryFile
	for _, vs := range g.videos {
		final := finalOf(g.plan, vs.incoming.Source)
		video, err := domain.NewVideo(vs.target.kind, domain.VideoSpec{
			Rel: final, Place: vs.place, Version: vs.version, Size: vs.size,
		}, now)
		if err != nil {
			return nil, err
		}
		files = append(files, video)
		for _, ss := range p.subtitles {
			if ss.video != vs || !ss.chosen || len(ss.issues) > 0 {
				continue
			}
			sub, err := domain.NewSubtitle(domain.SubtitleSpec{
				Rel:      finalOf(g.plan, libraryPath(p.entry, ss.rel)),
				VideoRel: final,
				Language: ss.language,
				Forced:   ss.choice.Forced,
				SDH:      ss.choice.SDH,
				Size:     ss.size,
			}, now)
			if err != nil {
				return nil, err
			}
			files = append(files, sub)
		}
	}
	return files, nil
}

// finalOf returns where a planned file of the placement goes: the target of its move.
func finalOf(plan domain.DecisionPlan, from string) string {
	for _, m := range plan.Place {
		if m.From == from {
			return m.To
		}
	}
	return ""
}
