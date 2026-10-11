package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// ErrFilteringDisabled means the filter is not wired: nothing is filtered or kept.
var ErrFilteringDisabled = errors.New("filtering is disabled")

// filtering holds the ports of the filter (RF-07): where what it found is recorded,
// and the rules that keep or discard each file.
type filtering struct {
	entries domain.FilterRepository
	rules   domain.FilterRules
}

// WithFiltering enables the filter of RF-07: what it finds is recorded in entries
// and each file is kept or discarded by rules. Without it, a content waits where it
// is. It must be set before RunExtraction.
func (s *Service) WithFiltering(entries domain.FilterRepository, rules domain.FilterRules) *Service {
	s.filter = &filtering{entries: entries, rules: rules}
	return s
}

// filterUpload takes a file uploaded on its own, which is not an archive, to
// filtering (uploaded → filtering, RF-10a). The file is filtered as it was uploaded.
func (s *Service) filterUpload(ctx context.Context, unit []*domain.UploadJob) error {
	if s.filter == nil {
		return nil
	}
	if err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.StartFiltering(now)
	}); err != nil {
		return s.abortedOr(ctx, err)
	}
	return s.filterUnit(ctx, unit, false)
}

// filterUnit filters the content of a work unit (RF-07): a file uploaded on its own,
// or the extracted content of an archive, or of a set of volumes. Every discarded file
// goes to the trash, once; a kept file stays where it is. Its jobs are then marked as
// filtered. A job that keeps nothing waits in filtering until its batch is complete,
// and then it is discarded or fails with no_video (RF-07, see settle.go). A job that
// keeps something waits for the step that moves it to No asignados (RF-08).
//
// What the filter found is recorded before anything moves, so a restart goes on
// with the same decisions and never trashes a file twice (RF-07, RF-10b).
func (s *Service) filterUnit(ctx context.Context, unit []*domain.UploadJob, extracted bool) error {
	if s.filter == nil {
		return nil
	}
	leader := leaderOf(unit[0])
	entries, err := s.filter.entries.Entries(ctx, leader)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		found, err := s.candidates(unit, extracted)
		if err != nil {
			return err
		}
		entries = s.decide(found)
		if err := s.filter.entries.Record(ctx, leader, entries); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if e.Kind.Kept() || e.Trashed {
			continue
		}
		if err := s.trashDiscard(ctx, unit, extracted, e); err != nil {
			return err
		}
		if err := s.filter.entries.MarkTrashed(ctx, leader, e.Path); err != nil {
			return err
		}
	}
	// Marked even when nothing is kept: the job then waits for its batch to be decided (RF-07).
	if err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.MarkFiltered(now)
	}); err != nil {
		return s.abortedOr(ctx, err)
	}
	s.queueKeptVideos(unit, extracted, entries) // read in the background; the job does not wait (RF-24)
	s.settleAll(ctx)
	return nil
}

// candidates lists the files of a work unit as the filter sees them: the file of an
// upload on its own (its path in the drop), or every file the extraction holds.
func (s *Service) candidates(unit []*domain.UploadJob, extracted bool) ([]domain.Candidate, error) {
	if !extracted {
		job := unit[0]
		return []domain.Candidate{{Path: originPath(job), Size: job.Size}}, nil
	}
	files, err := s.work.workspace.ExtractedFiles(leaderOf(unit[0]))
	if err != nil {
		// Nothing is decided on an unreadable folder: no file is discarded or failed,
		// and the unit is filtered again on the next start.
		return nil, fmt.Errorf("filter: list extracted content: %w", err)
	}
	out := make([]domain.Candidate, 0, len(files))
	for _, f := range files {
		out = append(out, domain.Candidate{Path: f.Path, Size: f.Size})
	}
	return out, nil
}

// decide gives each candidate its kind, sorted by path so that the work is the same
// on every run.
func (s *Service) decide(found []domain.Candidate) []domain.FilterEntry {
	out := make([]domain.FilterEntry, 0, len(found))
	for _, c := range found {
		out = append(out, domain.FilterEntry{Path: c.Path, Size: c.Size, Kind: s.filter.rules.Classify(c)})
	}
	slices.SortFunc(out, func(a, b domain.FilterEntry) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// trashDiscard sends one discarded file to the trash with its metadata (RF-07, RF-30).
// A file that is no longer where the filter found it went to the trash before a
// restart, when its record was not yet updated: nothing is sent twice.
func (s *Service) trashDiscard(ctx context.Context, unit []*domain.UploadJob, extracted bool, e domain.FilterEntry) error {
	job := unit[0]
	source, origin := s.work.workspace.ArchivePath(job.ID), originPath(job)
	if extracted {
		source = s.work.workspace.ExtractedPath(leaderOf(job), e.Path)
		origin = originPath(job) + "/" + e.Path
	}
	origin = strings.ReplaceAll(origin, `\`, "/")
	if !s.work.workspace.FileExists(source) {
		s.log.Warn("discarded file already gone", "unit", leaderOf(job), "path", e.Path)
		return nil
	}
	name := e.Path[strings.LastIndex(e.Path, "/")+1:]
	if err := s.work.trash.Put(ctx, TrashPut{
		Origin: TrashOriginUpload, Reason: TrashReasonFiltered,
		Name: name, BatchID: string(job.BatchID), RelativePath: origin,
		Source: source, Size: e.Size,
	}); err != nil {
		return fmt.Errorf("filter: trash %s: %w", e.Path, err)
	}
	return nil
}

// Kept returns what the filter kept of the job's content: the file of an upload
// on its own, or the files of the extracted archive, or of the set the volume belongs
// to. The next steps read the list from here (RF-07).
func (s *Service) Kept(ctx context.Context, id domain.JobID) ([]domain.FilterEntry, error) {
	if s.filter == nil {
		return nil, ErrFilteringDisabled
	}
	job, err := s.Job(ctx, id)
	if err != nil {
		return nil, err
	}
	entries, err := s.filter.entries.Entries(ctx, leaderOf(job))
	if err != nil {
		return nil, err
	}
	var kept []domain.FilterEntry
	for _, e := range entries {
		if e.Kind.Kept() {
			kept = append(kept, e)
		}
	}
	return kept, nil
}

// resumeFiltering settles the units that a restart interrupted in filtering (RF-07,
// RF-10b): each unit is filtered again from what was recorded. A set is one unit, so
// it is taken from its first volume only.
func (s *Service) resumeFiltering(ctx context.Context) {
	if s.filter == nil {
		return
	}
	defer s.settleAll(ctx) // the batches a restart left waiting are decided (RF-07)
	filtering, err := s.jobs.ByStatus(ctx, domain.StatusFiltering)
	if err != nil {
		s.log.Warn("filtering not resumed", "error", err)
		return
	}
	for _, job := range filtering {
		if job.FilterDone || (job.IsVolume() && job.VolumeIndex != 1) {
			continue
		}
		unit, err := s.unitOf(ctx, job)
		if err != nil {
			s.log.Warn("filtering not resumed", "job", job.ID, "error", err)
			continue
		}
		extracted := job.IsVolume() || s.work.workspace.ExtractionExists(leaderOf(job))
		if err := s.filterUnit(ctx, unit, extracted); err != nil && ctx.Err() == nil {
			s.log.Warn("filtering not finished", "job", job.ID, "error", err)
		}
	}
}
