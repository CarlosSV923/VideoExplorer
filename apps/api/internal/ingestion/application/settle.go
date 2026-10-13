package application

import (
	"context"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// RunSettler is the worker that decides the batches of the filter (RF-07, RF-08a). It runs
// until ctx ends, in the background: a restart decides the batches that a previous run left
// complete once the server listens, and no request waits for a decision. One worker only: the
// decisions are serialized by it. A pass that ctx stops leaves what it did not reach for the next
// start (the first pass of every start is over all the batches).
func (s *Service) RunSettler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.settle.wake:
		}
		covers, all, batches := s.settle.take()
		s.settlePass(ctx, all, batches)
		if ctx.Err() != nil {
			return
		}
		s.settle.finish(covers)
	}
}

// WaitSettled returns once every decision requested before the call is made, or when ctx ends.
// The requests never wait for it: it is a barrier for the tests and for whoever needs the
// decisions made at a given point.
func (s *Service) WaitSettled(ctx context.Context) error {
	return s.settle.wait(ctx)
}

// requestSettle asks for the decision of the batch of a job that ended (RF-07), or of every
// batch when the batch is not known. It never waits.
func (s *Service) requestSettle(batch domain.BatchID) {
	if batch == "" {
		s.settle.markAll()
		return
	}
	s.settle.mark(batch)
}

// settlePass decides the marked batches and, when all is set, every batch that has a job
// being filtered. A batch that fails stays for the next request.
func (s *Service) settlePass(ctx context.Context, all bool, marked []domain.BatchID) {
	if s.filter == nil {
		return
	}
	batches := marked
	if all {
		every, err := s.filteringBatches(ctx)
		if err != nil {
			s.log.Warn("batches not settled", "error", err)
			s.settle.retryAll()
			return
		}
		batches = append(batches, every...)
	}
	seen := make(map[domain.BatchID]bool, len(batches))
	for _, batch := range batches {
		if seen[batch] {
			continue
		}
		seen[batch] = true
		if ctx.Err() != nil {
			return // stopped: the batches not decided yet are decided at the next start
		}
		if err := s.settleBatch(ctx, batch); err != nil && ctx.Err() == nil {
			s.log.Warn("batch not settled", "batch", batch, "error", err)
			s.settle.retry(batch)
		}
	}
}

// filteringBatches returns each batch that has a job whose filter is done and that is still
// filtering, in the order of their first job.
func (s *Service) filteringBatches(ctx context.Context) ([]domain.BatchID, error) {
	waiting, err := s.jobs.ByStatus(ctx, domain.StatusFiltering)
	if err != nil {
		return nil, err
	}
	var out []domain.BatchID
	seen := map[domain.BatchID]bool{}
	for _, job := range waiting {
		if !job.FilterDone || seen[job.BatchID] {
			continue
		}
		seen[job.BatchID] = true
		out = append(out, job.BatchID)
	}
	return out, nil
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
	open, err := s.jobs.Unsettled(ctx, batch)
	if err != nil {
		return err
	}
	if open {
		return nil // still on its way: the job that ends last asks for the batch again
	}
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
