package application

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// MediaReader reads what a version label needs from a video: its frame and its audio tracks
// (RF-24). The composition root wires the ingestion's probe, cached per file.
type MediaReader interface {
	Media(ctx context.Context, path string) (domain.VersionMedia, error)
}

// versionOnDisk reads the version of a video the registry does not know, from its name (RF-24,
// RF-29). known is false when the name does not say it.
func versionOnDisk(kind domain.TitleKind, folder, rel string) (string, bool) {
	if kind == domain.KindSeries {
		return domain.SeriesVersionOf(rel)
	}
	return domain.MovieVersionOf(folder, rel)
}

// Occupants returns the videos of dups as occupants of a place (RF-23, RF-24): each with its
// subtitles, which are the files of its folder named after it, with their registry ids where the
// registry has them, and, when media is not nil, what its label reads. subtitleExts are the subtitle
// extensions with their dot (SUBTITLE_EXTENSIONS). Nothing is changed here.
func (d *DuplicateFinder) Occupants(ctx context.Context, folder string, dups domain.Duplicates, subtitleExts []string, media MediaReader) ([]domain.Occupant, error) {
	subIDs, err := d.registeredSubtitleIDs(ctx, folder)
	if err != nil {
		return nil, err
	}
	subExt := map[string]bool{}
	for _, e := range subtitleExts {
		subExt[strings.ToLower(e)] = true
	}
	out := make([]domain.Occupant, 0, len(dups.Files))
	for _, video := range dups.Files {
		dir := path.Dir(video.Rel)
		names, err := d.files.Names(d.files.LibraryPath(dir))
		if err != nil {
			return nil, fmt.Errorf("catalog: list %s: %w", dir, err)
		}
		var subs []domain.Existing
		for _, n := range names {
			if !subExt[strings.ToLower(path.Ext(n))] {
				continue
			}
			rel := dir + "/" + n
			if _, ok := domain.SubtitleSuffix(video.Rel, rel); !ok {
				continue
			}
			size, err := d.files.Size(d.files.LibraryPath(rel))
			if err != nil {
				return nil, fmt.Errorf("catalog: size of %s: %w", rel, err)
			}
			subs = append(subs, domain.Existing{Rel: rel, ID: subIDs[strings.ToLower(rel)], Size: size})
		}
		occupant := domain.Occupant{Video: video, Subtitles: subs}
		if media != nil {
			m, err := media.Media(ctx, d.files.LibraryPath(video.Rel))
			if err != nil {
				return nil, fmt.Errorf("catalog: media of %s: %w", video.Rel, err)
			}
			occupant.Media = m
		}
		out = append(out, occupant)
	}
	return out, nil
}

// registeredSubtitleIDs maps the path of each registered subtitle of the title, in lower case, to its id.
func (d *DuplicateFinder) registeredSubtitleIDs(ctx context.Context, folder string) (map[string]int64, error) {
	ids := map[string]int64{}
	title, found, err := d.repo.TitleByFolder(ctx, folder)
	if err != nil || !found {
		return ids, err
	}
	files, err := d.repo.Files(ctx, title.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Kind == domain.FileSubtitle {
			ids[strings.ToLower(f.Rel)] = f.ID
		}
	}
	return ids, nil
}

// Decide runs the change of a decision on a duplicate through the engine (RF-24, RNF-04): the
// replaced files go to the trash with their subtitles, the videos kept as versions take their label,
// and the new videos and subtitles go to their final paths. The engine never overwrites a file: a
// target that is taken fails the change, and nothing is left behind. The records of the change are
// written in its commit, together with the records of the registry it changes; extra adds the
// records the assignment itself writes (the title and its new files, RF-21), and may be nil. Skip
// changes nothing and records nothing (RF-24, Omitir).
func (e *Engine) Decide(ctx context.Context, plan domain.DecisionPlan, source string, extra func(Records) error) error {
	if plan.Decision == domain.DecisionSkip {
		return nil
	}
	return e.Do(ctx, "duplicate."+string(plan.Decision), source, decisionBuild(ctx, plan, e.now(), extra))
}

// decisionBuild plans the steps of a decision's plan and its records. The steps run in the order of
// the plan: the trash first, then the renamed versions, then the new files, so that a name the trash
// or a rename frees is free for the next step.
func decisionBuild(ctx context.Context, plan domain.DecisionPlan, now time.Time, extra func(Records) error) func(*Builder) error {
	return func(b *Builder) error {
		var (
			entry    domain.TrashEntry
			trashed  []int64
			hasTrash bool
		)
		if len(plan.Trash) > 0 {
			dir := randomName()
			files := make([]domain.TrashFile, 0, len(plan.Trash))
			for _, m := range plan.Trash {
				files = append(files, domain.TrashFile{Name: path.Base(m.From), Size: m.Size})
			}
			e, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonReplaced, dir,
				domain.Provenance{RelativePath: plan.Trash[0].From}, files, now)
			if err != nil {
				return err
			}
			if err := b.Mkdir(b.files.TrashPath(dir)); err != nil {
				return err
			}
			for i, m := range plan.Trash {
				if err := b.Move(b.files.LibraryPath(m.From), b.files.TrashPath(dir, files[i].Name)); err != nil {
					return fmt.Errorf("move to trash: %w", err)
				}
				if m.ID != 0 {
					trashed = append(trashed, m.ID)
				}
			}
			entry, hasTrash = e, true
		}
		for _, m := range plan.Renames {
			if err := b.Move(b.files.LibraryPath(m.From), b.files.LibraryPath(m.To)); err != nil {
				return err
			}
		}
		for _, m := range plan.Place {
			if err := b.Mkdir(b.files.LibraryPath(path.Dir(m.To))); err != nil {
				return err
			}
			if err := b.Move(b.files.LibraryPath(m.From), b.files.LibraryPath(m.To)); err != nil {
				return err
			}
		}
		b.Record(func(rec Records) error {
			if hasTrash {
				id, err := rec.InsertTrash(ctx, entry)
				if err != nil {
					return err
				}
				if err := rec.TrashLibraryFiles(ctx, trashed, id); err != nil {
					return err
				}
			}
			for _, m := range plan.Renames {
				if m.ID == 0 {
					continue
				}
				if err := rec.MoveLibraryFile(ctx, m.ID, m.To, m.Version); err != nil {
					return err
				}
			}
			if extra != nil {
				return extra(rec)
			}
			return nil
		})
		return nil
	}
}
