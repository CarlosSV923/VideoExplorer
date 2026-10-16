package application

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// TrashRestoreDeps are the registry and paths required to restore entries from the trash.
type TrashRestoreDeps struct {
	Repository   TrashRepository
	Library      LibraryRepository
	Files        Files
	Engine       *Engine
	Finder       *DuplicateFinder
	Media        MediaReader
	Roots        Roots
	SubtitleExts []string
	Now          func() time.Time
}

// TrashRestoreFilePlan is the preview of one video and its companion files.
type TrashRestoreFilePlan struct {
	File              string
	Destinations      []string
	Duplicates        []PlannedDuplicate
	Allowed           []domain.Decision
	Proposal          domain.Decision
	VersionProposal   string
	VersionIncomplete bool
}

// TrashRestorePlan lists proposed destinations and duplicate choices for a trash entry.
type TrashRestorePlan struct {
	EntryID int64
	Files   []TrashRestoreFilePlan
}

// TrashRestoreDecision is the RF-24 answer for one video in an entry.
type TrashRestoreDecision struct {
	File     string
	Decision domain.Decision
	Version  string
}

var (
	// ErrTrashRestoreDecisionRequired means a conflicting video has no per-video answer.
	ErrTrashRestoreDecisionRequired = errors.New("a restore decision is required")
	// ErrTrashRestoreCancelled means the user cancelled the whole restore operation.
	ErrTrashRestoreCancelled = errors.New("trash restore was cancelled")
)

// TrashRestore plans and commits the return of trashed items (RF-31, RF-31a).
type TrashRestore struct {
	d      TrashRestoreDeps
	now    func() time.Time
	subExt map[string]bool
}

// NewTrashRestore builds the restore use case over the library, its files and journaled engine.
func NewTrashRestore(d TrashRestoreDeps) *TrashRestore {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &TrashRestore{d: d, now: d.Now, subExt: extSet(d.SubtitleExts)}
}

func (r *TrashRestore) entry(ctx context.Context, id int64) (domain.TrashEntry, error) {
	entries, err := r.d.Repository.Entries(ctx)
	if err != nil {
		return domain.TrashEntry{}, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.TrashEntry{}, ErrTrashEntryNotFound
}

type restoreFile struct {
	title domain.LibraryTitle
	file  domain.LibraryFile
}

type restoreVideoPlan struct {
	video     restoreFile
	subs      []restoreFile
	incoming  domain.Incoming
	dups      domain.Duplicates
	occupants []domain.Occupant
	preview   duplicatePreview
	target    string
}

func (r *TrashRestore) videoPlan(ctx context.Context, entry domain.TrashEntry, group []restoreFile) (restoreVideoPlan, error) {
	var p restoreVideoPlan
	for i := range group {
		if group[i].file.Kind == domain.FileVideo {
			p.video = group[i]
		} else {
			p.subs = append(p.subs, group[i])
		}
	}
	if p.video.file.ID == 0 {
		return p, fmt.Errorf("catalog: trash entry has no video metadata")
	}
	target, err := r.libraryDestination(ctx, p.video.title, p.video.file)
	if err != nil {
		return p, err
	}
	p.target = target
	root := strings.SplitN(p.video.title.Folder, "/", 2)[0]
	place := p.video.file.Place
	title := domain.Title{Name: p.video.title.DiscTitle, Year: p.video.title.Year, TMDBID: p.video.title.TMDBID, IDTag: p.video.title.IDTag}
	chapter := ""
	if place.Episode {
		chapter = domain.ChapterTitleFromName(p.video.file.TrashRel, place.Season, place.First, place.Last, p.video.file.Version)
	}
	sourceRel := path.Join(".videoexplorer", "trash", entry.Dir, path.Base(p.video.file.TrashRel))
	p.incoming = domain.Incoming{Source: sourceRel, Kind: p.video.title.Kind, Root: root, Title: title, Place: place, ChapterTitle: chapter, Extension: path.Ext(p.video.file.TrashRel), Version: p.video.file.Version}
	for _, s := range p.subs {
		p.incoming.Subtitles = append(p.incoming.Subtitles, domain.IncomingSubtitle{Source: path.Join(".videoexplorer", "trash", entry.Dir, path.Base(s.file.TrashRel)), Language: s.file.Language, Forced: s.file.Forced, SDH: s.file.SDH, Extension: path.Ext(s.file.TrashRel)})
	}
	if r.d.Media != nil {
		p.incoming.Media, err = r.d.Media.Media(ctx, r.d.Files.LibraryPath(sourceRel))
		if err != nil {
			return p, err
		}
	}
	if r.d.Finder != nil {
		targetQuery := domain.Target{Kind: p.video.title.Kind}
		if p.video.title.Kind == domain.KindMovie {
			targetQuery.Parts = place.Part > 0
		} else {
			targetQuery.Place = place
		}
		p.dups, err = r.d.Finder.Find(ctx, p.video.title.Folder, targetQuery, sourceRel)
		if err != nil {
			return p, err
		}
		if len(p.dups.Files) > 0 {
			p.occupants, err = r.d.Finder.Occupants(ctx, p.video.title.Folder, p.dups, r.d.SubtitleExts, r.d.Media)
			if err != nil {
				return p, err
			}
			labels := domain.ProposeVersions(p.occupants, p.incoming)
			p.preview = previewOfDuplicates(p.dups, p.occupants, labels, false)
		}
	}
	return p, nil
}

func (r *TrashRestore) libraryFiles(ctx context.Context, entryID int64) ([]restoreFile, error) {
	titles, err := r.d.Library.Titles(ctx)
	if err != nil {
		return nil, err
	}
	var out []restoreFile
	for _, title := range titles {
		files, err := r.d.Library.Files(ctx, title.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.TrashEntryID == entryID {
				out = append(out, restoreFile{title: title, file: f})
			}
		}
	}
	return out, nil
}

func (r *TrashRestore) libraryDestination(ctx context.Context, t domain.LibraryTitle, f domain.LibraryFile) (string, error) {
	old := f.TrashRel
	if folderOfTitlePath(old) == strings.ToLower(t.Folder) {
		return old, nil
	}
	root := strings.SplitN(t.Folder, "/", 2)[0]
	title := domain.Title{Name: t.DiscTitle, Year: t.Year, TMDBID: t.TMDBID, IDTag: t.IDTag}
	var video string
	if f.Kind == domain.FileVideo {
		if t.Kind == domain.KindMovie {
			var err error
			video, err = domain.MoviePath(domain.Movie{Root: root, Title: title, Version: f.Version, Part: f.Place.Part, Extension: path.Ext(old)})
			if err != nil {
				return "", err
			}
		} else {
			chapter := domain.ChapterTitleFromName(old, f.Place.Season, f.Place.First, f.Place.Last, f.Version)
			var err error
			video, err = domain.EpisodePath(domain.Episode{Root: root, Title: title, Season: f.Place.Season, First: f.Place.First, Last: f.Place.Last, ChapterTitle: chapter, Version: f.Version, Extension: path.Ext(old)})
			if err != nil {
				return "", err
			}
		}
		return video, nil
	}
	// A subtitle follows its video's current §5 name, even if that title was renamed in the meantime.
	// Find the retained video row to use its place/version.
	videoFile, err := r.videoForSubtitle(ctx, t, f)
	if err != nil {
		return "", err
	}
	videoPath, err := r.libraryDestination(ctx, t, videoFile)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(videoPath, videoFile.TrashRel) {
		return old, nil
	}
	return domain.SubtitlePath(videoPath, domain.Subtitle{Language: f.Language, Forced: f.Forced, SDH: f.SDH, Extension: path.Ext(old)})
}

func folderOfTitlePath(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(strings.Join(parts[:2], "/"))
}

func (r *TrashRestore) videoForSubtitle(ctx context.Context, title domain.LibraryTitle, sub domain.LibraryFile) (domain.LibraryFile, error) {
	files, err := r.d.Library.Files(ctx, sub.TitleID)
	if err != nil {
		return domain.LibraryFile{}, err
	}
	for _, f := range files {
		if f.Kind == domain.FileVideo && strings.EqualFold(f.TrashRel, sub.VideoRel) {
			return f, nil
		}
	}
	place, ok := domain.PlaceOfName(title.Kind, sub.VideoRel)
	if !ok {
		return domain.LibraryFile{}, fmt.Errorf("catalog: subtitle video %s is missing", sub.VideoRel)
	}
	version := ""
	if title.Kind == domain.KindSeries {
		version, _ = domain.SeriesVersionOf(sub.VideoRel)
	} else {
		version, _ = domain.MovieVersionOf(title.Folder, sub.VideoRel)
	}
	return domain.LibraryFile{ID: sub.VideoID, TitleID: sub.TitleID, Kind: domain.FileVideo, Place: place, Version: version, TrashRel: sub.VideoRel}, nil
}

// Plan previews restoring an entry without changing files or records (RF-31, RF-31a).
func (r *TrashRestore) Plan(ctx context.Context, id int64) (TrashRestorePlan, error) {
	e, err := r.entry(ctx, id)
	if err != nil {
		return TrashRestorePlan{}, err
	}
	p := TrashRestorePlan{EntryID: id}
	if e.Origin != domain.TrashOriginLibrary {
		rel, err := r.unassignedRel(e)
		if err != nil {
			return p, err
		}
		for _, f := range e.Files {
			p.Files = append(p.Files, TrashRestoreFilePlan{File: f.Name, Destinations: []string{path.Join(domain.UnassignedDir, rel)}})
		}
		return p, nil
	}
	files, err := r.libraryFiles(ctx, id)
	if err != nil {
		return p, err
	}
	for _, group := range groupRestoreFiles(files) {
		if !restoreGroupHasVideo(group) {
			row := TrashRestoreFilePlan{File: group[0].file.TrashRel}
			for _, rf := range group {
				dest, err := r.libraryDestination(ctx, rf.title, rf.file)
				if err != nil {
					return p, err
				}
				row.Destinations = append(row.Destinations, dest)
			}
			p.Files = append(p.Files, row)
			continue
		}
		vp, err := r.videoPlan(ctx, e, group)
		if err != nil {
			return p, err
		}
		row := TrashRestoreFilePlan{File: vp.video.file.TrashRel, Destinations: []string{vp.target}}
		for _, sub := range vp.subs {
			dest, err := r.libraryDestination(ctx, sub.title, sub.file)
			if err != nil {
				return p, err
			}
			row.Destinations = append(row.Destinations, dest)
		}
		row.Duplicates, row.Allowed, row.Proposal = vp.preview.duplicates, withoutSkip(vp.preview.allowed), vp.preview.proposal
		row.VersionProposal, row.VersionIncomplete = vp.preview.versionProposal, vp.preview.versionIncomplete
		if len(row.Allowed) > 0 {
			row.Allowed = append(row.Allowed, domain.Decision("cancel"))
		}
		if row.Proposal == domain.DecisionSkip {
			row.Proposal = domain.DecisionReplace
		}
		p.Files = append(p.Files, row)
	}
	return p, nil
}

func restoreGroupHasVideo(group []restoreFile) bool {
	for _, rf := range group {
		if rf.file.Kind == domain.FileVideo {
			return true
		}
	}
	return false
}

func (r *TrashRestore) unassignedRel(e domain.TrashEntry) (string, error) {
	if err := domain.CheckLibraryRel(e.RelativePath); err != nil {
		return "", err
	}
	rel := e.RelativePath
	parts := strings.Split(rel, "/")
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	names, err := r.d.Files.Names(r.d.Files.UnassignedPath())
	if err != nil {
		return "", err
	}
	taken := domain.NewNameSet(names...)
	if dir != "" {
		parts[0] = domain.FreeName(parts[0], false, taken)
		return strings.Join(parts, "/"), nil
	}
	return domain.FreeName(path.Base(rel), true, taken), nil
}

func groupRestoreFiles(files []restoreFile) [][]restoreFile {
	byVideo := map[string][]restoreFile{}
	var order []string
	for _, rf := range files {
		key := rf.file.TrashRel
		if rf.file.Kind == domain.FileSubtitle {
			key = rf.file.VideoRel
		}
		key = strings.ToLower(key)
		if _, ok := byVideo[key]; !ok {
			order = append(order, key)
		}
		byVideo[key] = append(byVideo[key], rf)
	}
	out := make([][]restoreFile, 0, len(order))
	for _, key := range order {
		out = append(out, byVideo[key])
	}
	return out
}

// Commit restores an entry and applies each video's RF-24 decision in one engine operation.
func (r *TrashRestore) Commit(ctx context.Context, id int64, choices []TrashRestoreDecision) ([]string, error) {
	e, err := r.entry(ctx, id)
	if err != nil {
		return nil, err
	}
	returned := []string{}
	err = r.d.Engine.Do(ctx, "trash.restore", fmt.Sprintf("trash:%d", id), func(b *Builder) error {
		if e.Origin != domain.TrashOriginLibrary {
			// Uploads, archives, filtered files and Samba entries return beneath their recorded path.
			rel, err := r.unassignedRel(e)
			if err != nil {
				return err
			}
			dir := path.Dir(rel)
			if dir == "." {
				dir = ""
			}
			dest := path.Join(domain.UnassignedDir, rel)
			if err := b.Mkdir(path.Dir(b.files.LibraryPath(dest))); err != nil {
				return err
			}
			for _, f := range e.Files {
				target := dest
				if dir != "" {
					target = path.Join(domain.UnassignedDir, dir, f.Name)
				}
				source := b.files.TrashPath(e.Dir, f.Name)
				if err := b.Move(source, b.files.LibraryPath(target)); err != nil {
					return err
				}
				returned = append(returned, target)
			}
			b.RemoveDir(b.files.TrashPath(e.Dir))
			b.Record(func(rec Records) error { return rec.DeleteTrashEntry(ctx, id) })
			return nil
		}
		files, err := r.libraryFiles(ctx, id)
		if err != nil {
			return err
		}
		choiceByFile := map[string]TrashRestoreDecision{}
		for _, choice := range choices {
			key := strings.ToLower(choice.File)
			if _, exists := choiceByFile[key]; exists {
				return fmt.Errorf("catalog: duplicate restore decision for %s", choice.File)
			}
			choiceByFile[key] = choice
		}
		type applied struct {
			group    []restoreFile
			plan     domain.DecisionPlan
			incoming domain.Incoming
			decision domain.Decision
			trash    *domain.TrashEntry
		}
		var committed []applied
		for _, group := range groupRestoreFiles(files) {
			if !restoreGroupHasVideo(group) {
				dp := domain.DecisionPlan{}
				for i := range group {
					dest, err := r.libraryDestination(ctx, group[i].title, group[i].file)
					if err != nil {
						return err
					}
					if group[i].file.Kind == domain.FileSubtitle {
						video, err := r.videoForSubtitle(ctx, group[i].title, group[i].file)
						if err != nil {
							return err
						}
						videoDest, err := r.libraryDestination(ctx, group[i].title, video)
						if err != nil {
							return err
						}
						group[i].file.VideoRel = videoDest
					}
					source := path.Join(".videoexplorer", "trash", e.Dir, path.Base(group[i].file.TrashRel))
					if err := b.Mkdir(path.Dir(b.files.LibraryPath(dest))); err != nil {
						return err
					}
					if err := b.Move(b.files.LibraryPath(source), b.files.LibraryPath(dest)); err != nil {
						return err
					}
					dp.Place = append(dp.Place, domain.Move{From: source, To: dest})
					returned = append(returned, dest)
				}
				committed = append(committed, applied{group: group, plan: dp})
				continue
			}
			vp, err := r.videoPlan(ctx, e, group)
			if err != nil {
				return err
			}
			var dp domain.DecisionPlan
			decision := domain.DecisionReplace
			if len(vp.dups.Files) > 0 {
				choice, ok := choiceByFile[strings.ToLower(vp.video.file.TrashRel)]
				if !ok {
					return ErrTrashRestoreDecisionRequired
				}
				delete(choiceByFile, strings.ToLower(vp.video.file.TrashRel))
				decision = choice.Decision
				if decision == "cancel" {
					return ErrTrashRestoreCancelled
				}
				if decision != domain.DecisionReplace && decision != domain.DecisionKeepBoth {
					return fmt.Errorf("catalog: invalid restore decision %q", decision)
				}
				if decision == domain.DecisionKeepBoth {
					vp.incoming.Version = choice.Version
				} else {
					vp.incoming.Version = ""
				}
				dp, err = domain.PlanDecision(decision, vp.occupants, vp.dups.Exclusive, []domain.Incoming{vp.incoming})
				if err != nil {
					return err
				}
				if decision == domain.DecisionReplace {
					dp.Place = []domain.Move{{From: vp.incoming.Source, To: vp.target, Version: vp.video.file.Version}}
					for _, sub := range vp.incoming.Subtitles {
						target, err := domain.SubtitlePath(vp.target, domain.Subtitle{Language: sub.Language, Forced: sub.Forced, SDH: sub.SDH, Extension: sub.Extension})
						if err != nil {
							return err
						}
						dp.Place = append(dp.Place, domain.Move{From: sub.Source, To: target})
					}
				}
				var trashEntry *domain.TrashEntry
				if len(dp.Trash) > 0 {
					now := r.now()
					trashFiles := make([]domain.TrashFile, 0, len(dp.Trash))
					trashedIDs := make([]int64, 0, len(dp.Trash))
					for _, m := range dp.Trash {
						trashFiles = append(trashFiles, domain.TrashFile{Name: path.Base(m.From), Size: m.Size})
						if m.ID != 0 {
							trashedIDs = append(trashedIDs, m.ID)
						}
					}
					te, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonReplaced, randomName(), domain.Provenance{RelativePath: dp.Trash[0].From}, trashFiles, now)
					if err != nil {
						return err
					}
					trashEntry = &te
					if err := b.Mkdir(b.files.TrashPath(te.Dir)); err != nil {
						return err
					}
					for i, m := range dp.Trash {
						if err := b.Move(b.files.LibraryPath(m.From), b.files.TrashPath(te.Dir, trashFiles[i].Name)); err != nil {
							return err
						}
					}
					_ = trashedIDs
				}
				for _, m := range dp.Renames {
					if err := b.Move(b.files.LibraryPath(m.From), b.files.LibraryPath(m.To)); err != nil {
						return err
					}
				}
				for _, m := range dp.Place {
					if err := b.Mkdir(path.Dir(b.files.LibraryPath(m.To))); err != nil {
						return err
					}
					if err := b.Move(b.files.LibraryPath(m.From), b.files.LibraryPath(m.To)); err != nil {
						return err
					}
					returned = append(returned, m.To)
				}
				committed = append(committed, applied{group: group, plan: dp, incoming: vp.incoming, decision: decision, trash: trashEntry})
			} else {
				subPaths := map[string]string{}
				for _, sub := range vp.incoming.Subtitles {
					dest, err := domain.SubtitlePath(vp.target, domain.Subtitle{Language: sub.Language, Forced: sub.Forced, SDH: sub.SDH, Extension: sub.Extension})
					if err != nil {
						return err
					}
					subPaths[sub.Source] = dest
				}
				moves := []domain.Move{{From: vp.incoming.Source, To: vp.target, Version: vp.video.file.Version}}
				for src, dst := range subPaths {
					moves = append(moves, domain.Move{From: src, To: dst})
				}
				for _, m := range moves {
					if err := b.Mkdir(path.Dir(b.files.LibraryPath(m.To))); err != nil {
						return err
					}
					if err := b.Move(b.files.LibraryPath(m.From), b.files.LibraryPath(m.To)); err != nil {
						return err
					}
					returned = append(returned, m.To)
					dp.Place = append(dp.Place, m)
				}
				committed = append(committed, applied{group: group, plan: dp, incoming: vp.incoming, decision: decision})
			}
		}
		if len(choiceByFile) > 0 {
			return fmt.Errorf("catalog: restore decision names an unknown video")
		}
		b.RemoveDir(b.files.TrashPath(e.Dir))
		b.Record(func(rec Records) error {
			for _, done := range committed {
				if done.trash != nil {
					tid, err := rec.InsertTrash(ctx, *done.trash)
					if err != nil {
						return err
					}
					var ids []int64
					for _, m := range done.plan.Trash {
						if m.ID != 0 {
							ids = append(ids, m.ID)
						}
					}
					if err := rec.TrashLibraryFiles(ctx, ids, tid); err != nil {
						return err
					}
				}
				for _, m := range done.plan.Renames {
					if m.ID != 0 {
						if err := rec.MoveLibraryFile(ctx, m.ID, m.To, m.Version); err != nil {
							return err
						}
					}
				}
				placed := map[string]domain.Move{}
				for _, m := range done.plan.Place {
					placed[strings.ToLower(m.From)] = m
				}
				for _, rf := range done.group {
					source := path.Join(".videoexplorer", "trash", e.Dir, path.Base(rf.file.TrashRel))
					m, ok := placed[strings.ToLower(source)]
					if !ok {
						return fmt.Errorf("catalog: restore target for %s is missing", rf.file.TrashRel)
					}
					rf.file.Rel = m.To
					rf.file.TrashEntryID = 0
					if rf.file.Kind == domain.FileSubtitle {
						videoSource := path.Join(".videoexplorer", "trash", e.Dir, path.Base(rf.file.VideoRel))
						if vm, ok := placed[strings.ToLower(videoSource)]; ok {
							rf.file.VideoRel = vm.To
						}
					} else {
						rf.file.Version = m.Version
					}
					if err := rec.RestoreLibraryFile(ctx, rf.file); err != nil {
						return err
					}
				}
			}
			return rec.DeleteTrashEntry(ctx, id)
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return returned, nil
}
