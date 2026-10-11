package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// AbandonedAfter is how long an upload may stay idle before it is purged (RF-09, RF-10b).
const AbandonedAfter = 24 * time.Hour

// ListLimit is how many jobs GET /jobs returns, the most recent first (RF-10b).
const ListLimit = 100

// missingGrace protects an upload that was just admitted: tus writes its staged
// file right after admission, so a purge does not call a job without a file
// missing until it is older than this.
const missingGrace = time.Minute

// Service implements the ingestion use cases on the batch and job repositories.
type Service struct {
	batches domain.BatchRepository
	jobs    domain.JobRepository
	pub     Publisher
	staging Staging
	log     *slog.Logger
	now     func() time.Time
	newID   func() string
	// mu serializes the changes of existing jobs (progress, completion,
	// cancellation, purge), so that a read-modify-write never overwrites another.
	mu sync.Mutex
	// work is the extraction queue; nil until WithExtraction (RF-04).
	work *extraction
}

var _ Uploads = (*Service)(nil)

// NewService builds the service. pub receives every job change. log and now may
// be nil (a discarding logger and time.Now). Until WithStaging is called the
// staging area is empty.
func NewService(batches domain.BatchRepository, jobs domain.JobRepository, pub Publisher, log *slog.Logger, now func() time.Time) *Service {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if now == nil {
		now = time.Now
	}
	return &Service{batches: batches, jobs: jobs, pub: pub, staging: noStaging{}, log: log, now: now, newID: randomID}
}

// WithStaging sets the staging area that the purge inspects. It is set after
// the tus adapter exists, because the adapter needs the service first.
func (s *Service) WithStaging(staging Staging) *Service {
	if staging != nil {
		s.staging = staging
	}
	return s
}

// noStaging is the staging area of a library that cannot take uploads: nothing is staged.
type noStaging struct{}

func (noStaging) Inspect(context.Context, domain.JobID) (StagedUpload, error) {
	return StagedUpload{}, nil
}

func (noStaging) Remove(context.Context, domain.JobID) error { return nil }

func (noStaging) IDs(context.Context) ([]domain.JobID, error) { return nil, nil }

// randomID returns 16 random bytes in URL-safe base64 (22 characters), which
// is valid both as a batch id and as a tus upload id.
func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// CreateBatch starts a batch of files dropped together (RF-03). The name is
// provisional and may be empty.
func (s *Service) CreateBatch(ctx context.Context, name string, relation domain.Relation) (*domain.Batch, error) {
	batch, err := domain.NewBatch(domain.BatchID(s.newID()), name, relation, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.batches.Create(ctx, batch); err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	return batch, nil
}

// Admit implements Uploads. The upload must name a batch that exists
// (domain.ErrBatchNotFound otherwise, also for an empty batch id).
func (s *Service) Admit(ctx context.Context, a Admission) (domain.JobID, error) {
	if _, err := s.batches.Get(ctx, domain.BatchID(a.Meta.BatchID)); err != nil {
		return "", err
	}
	job, err := domain.NewUploadJob(domain.JobID(s.newID()), a.Meta, a.Size, s.now())
	if err != nil {
		return "", err
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return "", fmt.Errorf("create upload job: %w", err)
	}
	if err := s.announce(ctx, job); err != nil {
		return "", err
	}
	return job.ID, nil
}

// UploadProgress implements Uploads. Progress of a job that already ended is ignored.
func (s *Service) UploadProgress(ctx context.Context, id domain.JobID, received int64) error {
	return s.mutate(ctx, id, func(j *domain.UploadJob) (bool, error) {
		return j.RecordProgress(received, s.now()), nil
	})
}

// Completed implements Uploads. A job that the purge already recorded as
// uploaded stays as it is: the tus endpoint may report the same upload a moment later.
func (s *Service) Completed(ctx context.Context, id domain.JobID) error {
	changed, err := s.mutateChanged(ctx, id, func(j *domain.UploadJob) (bool, error) {
		if j.Status == domain.StatusUploaded {
			return false, nil
		}
		return true, j.MarkUploaded(s.now())
	})
	if err != nil {
		return err
	}
	if changed {
		return s.afterUpload(ctx, id) // an archive is extracted from here on (RF-04); a volume waits for its set (RF-03)
	}
	return nil
}

// Terminated implements Uploads. A job that already ended keeps its state. The
// browser deleting a finished upload cancels its work too (RF-09a): an extraction
// in progress stops and what it extracted is removed.
func (s *Service) Terminated(ctx context.Context, id domain.JobID) error {
	group := ""
	if job, err := s.jobs.Get(ctx, id); err == nil {
		group = job.VolumeGroup
	}
	changed, err := s.mutateChanged(ctx, id, func(j *domain.UploadJob) (bool, error) {
		if j.Status.Terminal() {
			return false, nil
		}
		return true, j.Cancel(s.now())
	})
	if err != nil {
		return err
	}
	if changed {
		if group != "" {
			s.cancelSet(ctx, group, id) // a claimed set is one work (RF-03)
		}
		s.abandonExtraction(id)
	}
	return nil
}

// Job returns one upload job.
func (s *Service) Job(ctx context.Context, id domain.JobID) (*domain.UploadJob, error) {
	return s.jobs.Get(ctx, id)
}

// List returns the most recent jobs with their batches, newest first (RF-10b).
func (s *Service) List(ctx context.Context) ([]domain.JobView, error) {
	return s.jobs.Recent(ctx, ListLimit)
}

// PurgeResult summarizes what a reconciliation or a purge changed.
type PurgeResult struct {
	// Completed counts uploads whose bytes all arrived and that were recorded as uploaded.
	Completed int
	// Missing counts uploading jobs whose file is gone; they failed with upload_missing.
	Missing int
	// Abandoned counts uploads idle for AbandonedAfter; they failed with upload_abandoned.
	Abandoned int
	// Orphans counts staged files that no job knows; they were deleted.
	Orphans int
	// PartsMissing counts volumes that waited for their set for AbandonedAfter; they failed with parts_missing (RF-10c).
	PartsMissing int
}

// Recover reconciles the jobs a previous run left behind (RF-10b). It runs once
// at start, before the upload endpoint accepts anything, so no upload is in
// flight and no grace is needed.
func (s *Service) Recover(ctx context.Context) (PurgeResult, error) {
	var res PurgeResult
	err := s.reconcile(ctx, s.now(), &res)
	return res, err
}

// Purge reconciles the jobs, fails the uploads idle for AbandonedAfter (deleting
// their partial files) and deletes the staged files that no job knows (RF-09,
// RF-10b). Deleting a partial file releases the bytes it reserved (RF-09a).
func (s *Service) Purge(ctx context.Context) (PurgeResult, error) {
	var res PurgeResult
	now := s.now()
	if err := s.reconcile(ctx, now.Add(-missingGrace), &res); err != nil {
		return res, err
	}

	cutoff := now.Add(-AbandonedAfter)
	stale, err := s.jobs.Stale(ctx, domain.StatusUploading, cutoff)
	if err != nil {
		return res, err
	}
	for _, job := range stale {
		changed, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
			if j.Status != domain.StatusUploading || !j.UpdatedAt.Before(cutoff) {
				return false, nil
			}
			if err := s.staging.Remove(ctx, j.ID); err != nil {
				return false, fmt.Errorf("remove partial upload %s: %w", j.ID, err)
			}
			return true, j.Fail(domain.FailureUploadAbandoned, now)
		})
		if err != nil {
			// The job stays uploading and the next purge tries again; the others go on.
			s.log.Warn("abandoned upload not purged", "upload", job.ID, "error", err)
			continue
		}
		if changed {
			res.Abandoned++
		}
	}

	// The staged ids are listed before the jobs are looked up. An upload admitted
	// in between has its job in the database before its file is written, so it
	// can never be taken for an orphan.
	missing, err := s.partsMissing(ctx, cutoff)
	if err != nil {
		return res, err
	}
	res.PartsMissing = missing

	ids, err := s.staging.IDs(ctx)
	if err != nil {
		return res, err
	}
	for _, id := range ids {
		exists, err := s.jobs.Exists(ctx, id)
		if err != nil {
			return res, err
		}
		if exists {
			continue
		}
		if err := s.staging.Remove(ctx, id); err != nil {
			s.log.Warn("orphan staged file not removed", "upload", id, "error", err)
			continue
		}
		res.Orphans++
	}
	return res, nil
}

// reconcile settles the uploading jobs against the staging area. A job whose
// bytes all arrived becomes uploaded, and one still arriving takes the count of
// its file. A job whose file is gone, and that was created before missingBefore,
// fails with upload_missing.
func (s *Service) reconcile(ctx context.Context, missingBefore time.Time, res *PurgeResult) error {
	uploading, err := s.jobs.ByStatus(ctx, domain.StatusUploading)
	if err != nil {
		return err
	}
	for _, job := range uploading {
		staged, err := s.staging.Inspect(ctx, job.ID)
		if err != nil {
			// One unreadable upload must not stop the others from being settled.
			s.log.Warn("upload not inspected", "upload", job.ID, "error", err)
			continue
		}
		switch {
		case staged.Exists && staged.Offset >= staged.Size:
			changed, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
				if j.Status != domain.StatusUploading {
					return false, nil
				}
				return true, j.MarkUploaded(s.now())
			})
			if err != nil {
				return err
			}
			if changed {
				res.Completed++
				if err := s.afterUpload(ctx, job.ID); err != nil {
					s.log.Warn("upload not carried on", "upload", job.ID, "error", err)
				}
			}
		case staged.Exists:
			// Still arriving: the staged file tells how far it got. The last progress
			// event may not have been stored before the process stopped, so the count
			// follows the file, the offset the browser resumes from.
			if _, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
				return j.ReconcileReceived(staged.Offset), nil
			}); err != nil {
				return err
			}
		case !job.CreatedAt.After(missingBefore):
			changed, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
				if j.Status != domain.StatusUploading {
					return false, nil
				}
				return true, j.Fail(domain.FailureUploadMissing, s.now())
			})
			if err != nil {
				return err
			}
			if changed {
				res.Missing++
			}
		}
	}
	return nil
}

// RunPurge purges at start and then every interval until ctx ends (RF-09, RF-10b).
func (s *Service) RunPurge(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		res, err := s.Purge(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			s.log.Warn("upload purge failed", "error", err)
		case res != (PurgeResult{}):
			s.log.Info("upload purge", "completed", res.Completed, "missing", res.Missing,
				"abandoned", res.Abandoned, "orphans", res.Orphans)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// mutate applies fn to a job under the service lock, saves it and announces the
// change when fn reports one.
func (s *Service) mutate(ctx context.Context, id domain.JobID, fn func(*domain.UploadJob) (bool, error)) error {
	_, err := s.mutateChanged(ctx, id, fn)
	return err
}

// mutateChanged is mutate that also reports whether the job changed.
func (s *Service) mutateChanged(ctx context.Context, id domain.JobID, fn func(*domain.UploadJob) (bool, error)) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return false, err
	}
	changed, err := fn(job)
	if err != nil || !changed {
		return false, err
	}
	if err := s.jobs.Save(ctx, job); err != nil {
		return false, fmt.Errorf("save upload job %s: %w", id, err)
	}
	if err := s.announce(ctx, job); err != nil {
		return false, err
	}
	return true, nil
}

// announce publishes the job, with its batch, to the event stream.
func (s *Service) announce(ctx context.Context, job *domain.UploadJob) error {
	batch, err := s.batches.Get(ctx, job.BatchID)
	if err != nil {
		return fmt.Errorf("batch of upload job %s: %w", job.ID, err)
	}
	s.pub.Publish(domain.JobView{Job: *job, Batch: *batch})
	return nil
}
