package application

import (
	"context"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// settleAll decides the batches that hold a job the filter emptied (RF-07). A batch is
// settled only when none of its jobs is still on its way; the decisions are serialized,
// so the last worker that finishes a batch decides for all of it.
func (s *Service) settleAll(ctx context.Context) {
	if s.filter == nil {
		return
	}
	s.settleMu.Lock()
	defer s.settleMu.Unlock()

	waiting, err := s.jobs.ByStatus(ctx, domain.StatusFiltering)
	if err != nil {
		s.log.Warn("batches not settled", "error", err)
		return
	}
	seen := map[domain.BatchID]bool{}
	for _, job := range waiting {
		if !job.FilterDone || seen[job.BatchID] {
			continue
		}
		seen[job.BatchID] = true
		if err := s.settleBatch(ctx, job.BatchID); err != nil && ctx.Err() == nil {
			s.log.Warn("batch not settled", "batch", job.BatchID, "error", err)
		}
	}
}

// settleBatch decides the jobs of a batch that the filter left with nothing to keep
// (RF-07). When the batch keeps something, they are discarded; when nothing is left of
// the batch at all, they fail with no_video. Nothing is decided while a job of the batch
// is still uploading, extracting, waiting for its parts or for its password, or still
// being filtered: those jobs decide later, when they end.
func (s *Service) settleBatch(ctx context.Context, batch domain.BatchID) error {
	jobs, err := s.jobs.ByBatch(ctx, batch)
	if err != nil {
		return err
	}
	keeps := false
	var empty []*domain.UploadJob
	keptBy := map[domain.JobID]bool{}
	for _, job := range jobs {
		switch job.Status {
		case domain.StatusDone:
			keeps = true
		case domain.StatusFiltering:
			if !job.FilterDone {
				return nil // still being filtered
			}
			leader := leaderOf(job)
			kept, known := keptBy[leader]
			if !known {
				if kept, err = s.hasKept(ctx, leader); err != nil {
					return err
				}
				keptBy[leader] = kept
			}
			if kept {
				keeps = true
			} else {
				empty = append(empty, job)
			}
		case domain.StatusUploading, domain.StatusUploaded, domain.StatusWaitingParts,
			domain.StatusExtracting, domain.StatusNeedsPassword:
			return nil // still on its way: the batch is not complete
		default:
			// failed, cancelled or discarded: ended by another cause, or by an earlier decision.
		}
	}
	if len(empty) == 0 {
		return nil
	}
	for _, unit := range groupByUnit(empty) {
		s.endEmpty(ctx, unit, keeps)
	}
	return nil
}

// hasKept reports whether filtering kept any file of the unit led by the job.
func (s *Service) hasKept(ctx context.Context, leader domain.JobID) (bool, error) {
	entries, err := s.filter.entries.Entries(ctx, leader)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Kind.Kept() {
			return true, nil
		}
	}
	return false, nil
}

// groupByUnit splits jobs into the work units they belong to: a set of volumes is one unit.
func groupByUnit(jobs []*domain.UploadJob) [][]*domain.UploadJob {
	var order []domain.JobID
	units := map[domain.JobID][]*domain.UploadJob{}
	for _, job := range jobs {
		leader := leaderOf(job)
		if _, seen := units[leader]; !seen {
			order = append(order, leader)
		}
		units[leader] = append(units[leader], job)
	}
	out := make([][]*domain.UploadJob, 0, len(order))
	for _, leader := range order {
		out = append(out, units[leader])
	}
	return out
}

// endEmpty ends the jobs of one unit that the filter emptied (RF-07): discarded when the
// batch keeps something, failed with no_video when nothing is left in the batch. What
// was extracted or staged is removed: the discarded files are in the trash already.
func (s *Service) endEmpty(ctx context.Context, unit []*domain.UploadJob, keeps bool) {
	s.removeExtraction(leaderOf(unit[0]))
	s.removeArchives(ctx, unit)
	var err error
	state := domain.StatusDiscarded
	if keeps {
		err = s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error { return j.Discard(now) })
	} else {
		state = domain.StatusFailed
		err = s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error { return j.Fail(domain.FailureNoVideo, now) })
	}
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("filtered job not ended", "job", unit[0].ID, "error", err)
		}
		return
	}
	s.log.Info("filtering decided", "job", unit[0].ID, "status", state, "batch", unit[0].BatchID)
}
