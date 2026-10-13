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
		if ctx.Err() != nil {
			return // stopped: the batches not decided yet are decided at the next start
		}
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
//
// Once the batch is complete, what it kept moves to No asignados (RF-08, RF-08a): see
// placeBatch. A job that failed or was cancelled on the way counts as ended: it does not
// move, and Reintentar moves it later.
func (s *Service) settleBatch(ctx context.Context, batch domain.BatchID) error {
	jobs, err := s.jobs.ByBatch(ctx, batch)
	if err != nil {
		return err
	}
	keeps := false
	var empty, keptJobs []*domain.UploadJob
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
			keptUnit, known := keptBy[leader]
			if !known {
				if keptUnit, err = s.hasKept(ctx, leader); err != nil {
					return err
				}
				keptBy[leader] = keptUnit
			}
			if keptUnit {
				keeps = true
				keptJobs = append(keptJobs, job)
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
	for _, unit := range groupByUnit(empty) {
		s.endEmpty(ctx, unit, keeps)
	}
	if len(keptJobs) == 0 || s.library == nil {
		return nil
	}
	b, err := s.batches.Get(ctx, batch)
	if err != nil {
		return err
	}
	return s.placeBatch(ctx, b, keptJobs)
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
// was extracted or staged is removed: the discarded files are in the trash already. An
// original of an archive is removed only once its move to the trash is done: if that move
// fails now, the unit fails with trash_failed and keeps everything (RF-08a).
func (s *Service) endEmpty(ctx context.Context, unit []*domain.UploadJob, keeps bool) {
	if s.extracted(unit[0]) {
		if err := s.trashOriginals(ctx, unit); err != nil {
			if ctx.Err() == nil {
				if ferr := s.failTrash(ctx, unit, err); ferr != nil && ctx.Err() == nil {
					s.log.Warn("trash failure not recorded", "job", unit[0].ID, "error", ferr)
				}
			}
			return
		}
	}
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
