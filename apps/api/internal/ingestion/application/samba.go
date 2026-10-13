package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// WithSamba sets the section that the compressed files Samba copied come from (RF-27a). Until it is
// set, nothing is adopted and no job of Samba is shown.
func (s *Service) WithSamba(section SambaSection) *Service {
	s.samba = section
	return s
}

// AdoptSamba takes the compressed files that Samba copied into No asignados, and runs them as uploads
// (RF-27a). Every settled file that starts with the magic bytes of an archive, or that is an old RAR
// volume, becomes a unit (domain.GroupSamba): its batch (origin samba, relation none) and its jobs are
// recorded first, then its files move into staging in one change of the library, and then its jobs go on
// as an upload that completed (RF-04 to RF-08a). A unit that cannot be taken stays where it is, and the
// next call decides again. It returns how many units it took.
//
// It is the operation that reading the section calls, and that the scan of RF-28 will call too. It does
// nothing when the extraction is off or no section is wired. Calls do not overlap.
func (s *Service) AdoptSamba(ctx context.Context) (int, error) {
	if s.samba == nil || s.work == nil {
		return 0, nil
	}
	s.adopting.Lock()
	defer s.adopting.Unlock()

	files, err := s.samba.Files(ctx)
	if err != nil {
		return 0, fmt.Errorf("ingestion: list samba copies: %w", err)
	}
	for i := range files {
		if files[i].Copying || files[i].Size <= 0 {
			continue
		}
		header, err := s.work.workspace.ReadHeader(files[i].Source, 8)
		if err != nil {
			// Gone or unreadable: it is not taken now, and the next reading decides again.
			s.log.Warn("samba copy not read", "file", files[i].Name, "error", err)
			files[i].Copying = true
			continue
		}
		_, files[i].Archive = domain.DetectArchive(header)
	}

	adopted := 0
	for _, u := range domain.GroupSamba(files) {
		if ctx.Err() != nil {
			break
		}
		if err := s.adoptUnit(ctx, u); err != nil {
			// A copy that changed under the change is not an error: the next reading takes it (RF-27a).
			if ctx.Err() == nil && !errors.Is(err, ErrSambaChanged) {
				s.log.Warn("samba copy not taken", "file", u.Files[0].Name, "error", err)
			}
			continue
		}
		adopted++
	}
	return adopted, nil
}

// adoptUnit takes one unit of Samba copies (RF-27a). The journal of the change is what makes the move
// safe: the jobs exist before the files move, so that a file in staging always has its job, and a
// restart finds either the file in staging with its job still uploading (recoverSamba finishes it) or
// the file back in the section with no file in staging (recoverSamba forgets the job).
func (s *Service) adoptUnit(ctx context.Context, u domain.SambaUnit) error {
	now := s.now()
	batch, err := domain.NewSambaBatch(domain.BatchID(s.newID()), domain.EntryBase(u.Files[0].Name), now)
	if err != nil {
		return err
	}
	if err := s.batches.Create(ctx, batch); err != nil {
		return fmt.Errorf("create samba batch: %w", err)
	}
	parts := 0
	if u.Set {
		parts = len(u.Files)
	}
	jobs := make([]*domain.UploadJob, 0, len(u.Files))
	moves := make([]SambaMove, 0, len(u.Files))
	for _, f := range u.Files {
		job, err := domain.NewSambaJob(domain.JobID(s.newID()), batch.ID, f.Name, f.SourcePath(), f.Size, parts, now)
		if err != nil {
			s.dropSamba(ctx, batch.ID, jobs)
			return err
		}
		if err := s.jobs.Create(ctx, job); err != nil {
			s.dropSamba(ctx, batch.ID, jobs)
			return fmt.Errorf("create samba job: %w", err)
		}
		if err := s.announce(ctx, job); err != nil {
			s.log.Warn("samba job not announced", "job", job.ID, "error", err)
		}
		jobs = append(jobs, job)
		moves = append(moves, SambaMove{Rel: f.Rel, Dst: s.work.workspace.ArchivePath(job.ID)})
	}

	if err := s.samba.Take(ctx, u.Entry, moves); err != nil {
		// A change that was undone moved nothing: the copies are where they were, and their jobs go.
		// A file already in staging keeps its jobs, and the restart finishes them (recoverSamba).
		if s.anyStaged(jobs) {
			return fmt.Errorf("take samba copy (kept for the restart): %w", err)
		}
		s.dropSamba(ctx, batch.ID, jobs)
		return fmt.Errorf("take samba copy: %w", err)
	}

	for _, job := range jobs {
		if _, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
			if j.Status != domain.StatusUploading {
				return false, nil
			}
			return true, j.MarkUploaded(s.now())
		}); err != nil {
			return fmt.Errorf("mark samba job %s uploaded: %w", job.ID, err)
		}
	}
	for _, job := range jobs {
		if err := s.afterUpload(ctx, job.ID); err != nil {
			s.log.Warn("samba job not carried on", "job", job.ID, "error", err)
		}
	}
	s.log.Info("samba copy taken", "batch", batch.ID, "files", len(jobs))
	return nil
}

// anyStaged reports whether a file of the jobs is already in staging.
func (s *Service) anyStaged(jobs []*domain.UploadJob) bool {
	for _, job := range jobs {
		if s.work.workspace.ArchiveExists(job.ID) {
			return true
		}
	}
	return false
}

// dropSamba forgets the jobs and the batch of an adoption that did not move its files (RF-27a).
// Failures are logged: a batch left without jobs is harmless and the next reading does not use it.
func (s *Service) dropSamba(ctx context.Context, batch domain.BatchID, jobs []*domain.UploadJob) {
	for _, job := range jobs {
		if err := s.jobs.Delete(ctx, job.ID); err != nil {
			s.log.Warn("samba job not forgotten", "job", job.ID, "error", err)
		}
	}
	s.forgetBatchIfEmpty(ctx, batch)
}

// forgetBatchIfEmpty deletes a batch that no job refers to any more.
func (s *Service) forgetBatchIfEmpty(ctx context.Context, batch domain.BatchID) {
	left, err := s.jobs.ByBatch(ctx, batch)
	if err != nil || len(left) > 0 {
		return
	}
	if err := s.batches.Delete(ctx, batch); err != nil {
		s.log.Warn("samba batch not forgotten", "batch", batch, "error", err)
	}
}

// recoverSamba finishes the adoptions a previous run left, before the server serves (RF-27a, RNF-04).
// A job of origin samba still uploading whose file is in staging belongs to a move that was committed:
// it is complete and goes on as an upload. Without its file the move did not commit (the journal undid
// it, before this runs), so the copy is still in the section: the job and its batch go, and the next
// reading takes the copy again. A job never gets there twice, because its batch is gone with it.
func (s *Service) recoverSamba(ctx context.Context) {
	if s.work == nil {
		return
	}
	uploading, err := s.jobs.ByStatus(ctx, domain.StatusUploading)
	if err != nil {
		s.log.Warn("samba copies not recovered", "error", err)
		return
	}
	for _, job := range uploading {
		if !job.IsSamba() {
			continue
		}
		if s.work.workspace.ArchiveExists(job.ID) {
			if _, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
				if j.Status != domain.StatusUploading {
					return false, nil
				}
				return true, j.MarkUploaded(s.now())
			}); err != nil {
				s.log.Warn("samba job not completed", "job", job.ID, "error", err)
				continue
			}
			if err := s.afterUpload(ctx, job.ID); err != nil {
				s.log.Warn("samba job not carried on", "job", job.ID, "error", err)
			}
			continue
		}
		if err := s.jobs.Delete(ctx, job.ID); err != nil {
			s.log.Warn("samba job not forgotten", "job", job.ID, "error", err)
			continue
		}
		s.forgetBatchIfEmpty(ctx, job.BatchID)
		s.log.Info("samba copy left in the section", "job", job.ID)
	}
}

// SambaJobs returns the jobs that the section shows for the copies of Samba (RF-27a): the job of each
// unit, which is the job of its first volume for a set, for as long as the unit has not ended in No
// asignados, the trash or a cancellation. A failed unit stays listed, so that its password or its retry
// can be sent with the jobs endpoints (RF-10c).
func (s *Service) SambaJobs(ctx context.Context) ([]domain.JobView, error) {
	open, err := s.jobs.OpenSamba(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.JobView
	for _, job := range open {
		if job.IsVolume() && job.VolumeIndex != 1 {
			continue
		}
		view, err := s.view(ctx, job)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// DismissSamba takes a failed copy of Samba off No asignados ("Quitar", RF-27a). The unit is the job and
// the jobs of its volumes: each is marked, once all of them can be (domain.ErrNotDismissable otherwise, and
// nothing changes). The job stays in the jobs history; the section stops listing it.
func (s *Service) DismissSamba(ctx context.Context, id domain.JobID) (domain.JobView, error) {
	job, err := s.Job(ctx, id)
	if err != nil {
		return domain.JobView{}, err
	}
	if !job.IsSamba() {
		return domain.JobView{}, fmt.Errorf("%w: not a copy of Samba", domain.ErrNotDismissable)
	}
	unit, err := s.jobs.ByBatch(ctx, job.BatchID)
	if err != nil {
		return domain.JobView{}, err
	}
	for _, j := range unit {
		if !j.Dismissable() {
			return domain.JobView{}, fmt.Errorf("%w: %s (%q)", domain.ErrNotDismissable, j.Status, j.Error)
		}
		if s.copyOf(j) != "" {
			// "Quitar" needs the copy in the trash: its move is retried at the start and at each purge (RF-27a).
			return domain.JobView{}, fmt.Errorf("%w: its copy is not in the trash yet", domain.ErrNotDismissable)
		}
	}
	for _, j := range unit {
		if _, err := s.mutateChanged(ctx, j.ID, func(x *domain.UploadJob) (bool, error) {
			if x.Dismissed {
				return false, nil
			}
			return true, x.Dismiss(s.now())
		}); err != nil {
			return domain.JobView{}, err
		}
	}
	return s.reloaded(ctx, id)
}

// retryDiscards sends again to the trash the copies of failed Samba jobs whose move did not finish (RF-27a).
// Only a unit that failed for a reason a retry cannot fix is looked at: the others keep their copy for
// Reintentar. It runs at the start, before the server serves, and at each purge, so that a move that failed
// is not left for good. A copy that is already in the trash is not moved twice.
func (s *Service) retryDiscards(ctx context.Context) {
	if s.work == nil {
		return
	}
	open, err := s.jobs.OpenSamba(ctx)
	if err != nil {
		s.log.Warn("samba copies not checked for the trash", "error", err)
		return
	}
	seen := map[domain.BatchID]bool{}
	for _, job := range open {
		if job.Status != domain.StatusFailed || job.Error.Retryable() || seen[job.BatchID] {
			continue
		}
		seen[job.BatchID] = true
		unit, err := s.jobs.ByBatch(ctx, job.BatchID)
		if err != nil {
			s.log.Warn("samba copy not checked for the trash", "batch", job.BatchID, "error", err)
			continue
		}
		if err := s.discardCopies(ctx, unit); err != nil && ctx.Err() == nil {
			s.log.Warn("samba copy still not in the trash", "batch", job.BatchID, "error", err)
		}
	}
}
