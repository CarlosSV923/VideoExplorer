package application

import (
	"context"
	"errors"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// ErrOriginalPending means the browser asked to delete an upload whose original archive still
// waits in staging for its move to the trash (RF-08a). The deletion is refused: the file is not
// deleted without its move, so the job is retried first.
var ErrOriginalPending = errors.New("the original of the upload waits for its move to the trash")

// extracted reports whether the content of a job comes from an extraction: a set of volumes, or
// an archive with an extraction folder. Only such a job has an original that is moved to the
// trash; a file uploaded on its own is the content itself.
func (s *Service) extracted(job *domain.UploadJob) bool {
	return s.work != nil && (job.IsVolume() || s.work.workspace.ExtractionExists(leaderOf(job)))
}

// awaitingTrash reports whether the original of a job waits in staging for its move to the
// trash: the extraction of its archive ended (the job is filtering, or failed with trash_failed)
// and the file is still there. An original that is still being extracted is not waiting for the
// trash: a deletion or a failure removes it as before.
func (s *Service) awaitingTrash(job *domain.UploadJob) bool {
	ended := job.Status == domain.StatusFiltering ||
		(job.Status == domain.StatusFailed && job.Error == domain.FailureTrashFailed)
	return ended && s.extracted(job) && s.work.workspace.FileExists(s.archivePath(job))
}

// keepsExtraction reports whether the extracted files of a job must survive a stop of its worker:
// the filter and the trash move read them again after the restart (RF-07, RF-08a).
func (s *Service) keepsExtraction(id domain.JobID) bool {
	job, err := s.jobs.Get(context.Background(), id)
	if err != nil {
		return false
	}
	return job.Status == domain.StatusFiltering ||
		(job.Status == domain.StatusFailed && job.Error == domain.FailureTrashFailed)
}

// failTrash ends a unit whose original did not reach the trash with trash_failed (RF-08a). The
// originals that did not move stay in staging, and the extraction stays too, so that Reintentar
// goes on from here. The other jobs of the batch are not touched.
func (s *Service) failTrash(ctx context.Context, unit []*domain.UploadJob, cause error) error {
	s.log.Error("original not sent to the trash", "job", unit[0].ID, "error", cause)
	return s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.Fail(domain.FailureTrashFailed, now)
	})
}

// trashAgain repeats the move to the trash of a job that failed with trash_failed (RF-08a): only
// the moves still pending are made, and the unit goes on. A unit that was filtered already goes
// on to the settlement of its batch; any other goes on to the filter (RF-07).
func (s *Service) trashAgain(ctx context.Context, job *domain.UploadJob) error {
	if job.Status != domain.StatusFiltering {
		return nil
	}
	unit, err := s.unitOf(ctx, job)
	if err != nil {
		return err
	}
	if err := s.trashOriginals(ctx, unit); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		err = s.failTrash(ctx, unit, err)
		s.settle.mark(unit[0].BatchID) // the failed unit ends again, which may let its batch be decided (RF-07)
		return s.abortedOr(ctx, err)
	}
	if job.FilterDone {
		s.settle.mark(unit[0].BatchID)
		return nil
	}
	return s.filterUnit(ctx, unit, true)
}

// retryTrash sends a job that failed with trash_failed back to filtering (RF-08a). The jobs of its
// unit go back together: a set is one work, and its volumes wait in the set's folder.
func (s *Service) retryTrash(ctx context.Context, job *domain.UploadJob) (domain.JobView, error) {
	unit, err := s.unitOf(ctx, job)
	if err != nil {
		return domain.JobView{}, ErrCannotRetry
	}
	if err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.RetryTrash(now)
	}); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return domain.JobView{}, ErrCannotRetry
		}
		return domain.JobView{}, err
	}
	s.work.enqueue(extractionRequest{id: leaderOf(job), trash: true})
	return s.reloaded(ctx, job.ID)
}

// Terminating is asked before the browser's deletion of an upload is carried out (RF-09a). The
// original of an archive whose extraction ended waits in staging for its move to the trash
// (RF-08a) while its job failed with trash_failed or is being filtered: such a deletion is
// refused with ErrOriginalPending, so that the original is never deleted without its move. Any
// other upload, and an upload this library does not know, is deleted as before.
func (s *Service) Terminating(ctx context.Context, id domain.JobID) error {
	if s.work == nil {
		return nil
	}
	job, err := s.jobs.Get(ctx, id)
	if errors.Is(err, domain.ErrJobNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if job.IsVolume() && job.VolumeGroup == "" {
		return nil // a volume still waiting for its set has no extraction
	}
	unit, err := s.unitOf(ctx, job)
	if err != nil {
		return err
	}
	for _, m := range unit {
		if s.awaitingTrash(m) {
			return ErrOriginalPending
		}
	}
	return nil
}
