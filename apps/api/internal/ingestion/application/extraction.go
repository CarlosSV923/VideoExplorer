package application

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// progressInterval is the least time between two progress writes of one archive (RF-10).
const progressInterval = time.Second

// queueSize is how many jobs may wait for an extraction worker. Jobs beyond it
// wait in a goroutine, so nothing is lost while the workers are busy.
const queueSize = 1024

// maxPasswordBytes bounds a password: it travels as an argument of 7zz (RF-04).
const maxPasswordBytes = 1024

var (
	// ErrNotWaitingForPassword means the job is not paused for a password (RF-04).
	ErrNotWaitingForPassword = errors.New("job is not waiting for a password")
	// ErrCannotRetry means the job is not a failure that admits a retry, or its archive is gone (RF-10c).
	ErrCannotRetry = errors.New("job cannot be retried")
	// ErrExtractionDisabled means the library cannot extract: there is no staging area to work in.
	ErrExtractionDisabled = errors.New("extraction is disabled: the library is not writable")
	// ErrInvalidPassword means the password is empty, too long or has a NUL byte.
	ErrInvalidPassword = errors.New("invalid archive password")
)

// extraction is the work queue of the archives: the jobs waiting for a worker
// and the extractions running now (RF-04 to RF-06, RF-10c).
type extraction struct {
	workspace Workspace
	archives  Extractor
	trash     Trash

	queue   chan extractionRequest
	stopped chan struct{} // closed when RunExtraction returns

	mu      sync.Mutex
	running map[domain.JobID]context.CancelFunc
	// pending holds a request that came for a job while a worker still had it:
	// it runs as soon as that worker is done, so it is never lost.
	pending map[domain.JobID]extractionRequest
}

// extractionRequest asks for a job to be extracted. For a set of volumes the
// job is the set's first volume, which runs the extraction of the whole set. The
// password is held only in memory, for this extraction; it is never stored or
// logged. started says the job is already in extracting (a retry, or a set that
// was just claimed), so the extraction does not start it again.
type extractionRequest struct {
	id       domain.JobID
	password string
	started  bool
	// refilter filters again a job that failed with filter_failed (RF-07, RF-08a).
	refilter bool
	// settle decides the batches of the filter again, so that a job that failed with move_failed
	// moves its content again (RF-08a).
	settle bool
	// trash moves again the originals of a job that failed with trash_failed, and goes on (RF-08a).
	trash bool
}

// WithExtraction enables the extraction of archives: where they wait and are
// extracted, the tool that opens them and the trash that receives their
// originals (RF-04, RF-06). It must be set before RunExtraction.
func (s *Service) WithExtraction(w Workspace, x Extractor, t Trash) *Service {
	s.work = &extraction{
		workspace: w, archives: x, trash: t,
		queue:   make(chan extractionRequest, queueSize),
		stopped: make(chan struct{}),
		running: map[domain.JobID]context.CancelFunc{},
		pending: map[domain.JobID]extractionRequest{},
	}
	return s
}

// RunExtraction runs n extraction workers (EXTRACT_CONCURRENCY) until ctx ends.
// The jobs that a restart interrupted are queued again first (RF-10a, RF-10b).
func (s *Service) RunExtraction(ctx context.Context, n int) {
	if s.work == nil {
		return
	}
	defer close(s.work.stopped)
	s.resumeExtractions(ctx)
	s.resumeFiltering(ctx)

	var wg sync.WaitGroup
	for range max(1, n) {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case r := <-s.work.queue:
					s.process(ctx, r)
				}
			}
		})
	}
	wg.Wait()
}

// enqueue schedules an extraction. It never blocks the caller (the tus endpoint
// calls it when an upload completes).
func (x *extraction) enqueue(r extractionRequest) {
	select {
	case x.queue <- r:
	default:
		go func() {
			select {
			case x.queue <- r:
			case <-x.stopped:
			}
		}()
	}
}

// enqueueJob schedules the extraction of a job, if extraction is enabled.
func (s *Service) enqueueJob(id domain.JobID) {
	if s.work != nil {
		s.work.enqueue(extractionRequest{id: id})
	}
}

// resumeExtractions queues again what a restart interrupted (RF-10a, RF-10b). An
// extraction in progress starts over: a single archive goes back to uploaded, a
// set goes back to waiting and is claimed again once it is complete. An uploaded
// archive is extracted. A job waiting for its password keeps waiting (RF-04).
func (s *Service) resumeExtractions(ctx context.Context) {
	extracting, err := s.jobs.ByStatus(ctx, domain.StatusExtracting)
	if err != nil {
		s.log.Warn("extractions not resumed", "error", err)
		return
	}
	for _, job := range extracting {
		if _, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
			if j.IsVolume() {
				return j.ResumeVolumeAfterRestart(s.now()), nil
			}
			return j.ResumeAfterRestart(s.now()), nil
		}); err != nil {
			s.log.Warn("extraction not resumed", "job", job.ID, "error", err)
		}
	}
	uploaded, err := s.jobs.ByStatus(ctx, domain.StatusUploaded)
	if err != nil {
		s.log.Warn("extractions not resumed", "error", err)
		return
	}
	for _, job := range uploaded {
		if err := s.afterUpload(ctx, job.ID); err != nil {
			s.log.Warn("upload not carried on", "job", job.ID, "error", err)
		}
	}
	waiting, err := s.jobs.ByStatus(ctx, domain.StatusWaitingParts)
	if err != nil {
		s.log.Warn("sets not checked", "error", err)
		return
	}
	for _, job := range waiting {
		if err := s.checkSet(ctx, job.ID); err != nil {
			s.log.Warn("set not checked", "job", job.ID, "error", err)
		}
	}
}

// process runs one request on a worker. A job is extracted by one worker at a
// time; work that the job was cancelled in the middle of leaves nothing behind.
func (s *Service) process(parent context.Context, r extractionRequest) {
	x := s.work
	ctx, cancel := context.WithCancel(parent)
	x.mu.Lock()
	if _, busy := x.running[r.id]; busy {
		x.pending[r.id] = r // a worker has the job: it gets this request when it is done
		x.mu.Unlock()
		cancel()
		return
	}
	x.running[r.id] = cancel
	x.mu.Unlock()
	defer func() {
		// A stopped worker leaves nothing half-extracted, but a job past its extraction keeps its
		// folder: the filter and the trash move read it again after the restart (RF-07, RF-08a).
		if ctx.Err() != nil && !s.keepsExtraction(r.id) {
			_ = x.workspace.RemoveExtraction(r.id)
		}
		cancel()
		x.mu.Lock()
		delete(x.running, r.id)
		next, again := x.pending[r.id]
		delete(x.pending, r.id)
		x.mu.Unlock()
		if again {
			x.enqueue(next)
		}
	}()

	if err := s.extract(ctx, r); err != nil && ctx.Err() == nil {
		s.log.Error("extraction not finished", "job", r.id, "error", err)
	}
}

// extract runs the steps of RF-04 to RF-06 for one job, or for the set it
// leads. The outcomes the user can act on end the job with their reason (RF-10c);
// an error returned here is not the archive's fault and leaves the job as it is,
// until the next start.
func (s *Service) extract(ctx context.Context, r extractionRequest) error {
	x := s.work
	job, err := s.Job(ctx, r.id)
	if err != nil {
		return err
	}
	if r.settle {
		s.settle.mark(job.BatchID)
		return nil
	}
	if r.trash {
		return s.trashAgain(ctx, job)
	}
	if r.refilter {
		return s.refilter(ctx, job)
	}
	if !extractable(job, r) {
		return nil
	}
	unit, err := s.unitOf(ctx, job)
	if err != nil {
		return err
	}
	if job.IsVolume() && job.VolumeIndex != 1 {
		return nil // a set is extracted from its first volume only (RF-03)
	}
	if !job.IsVolume() && domain.IsLegacyVolume(job.FileName) {
		if err := s.failRARSiblings(ctx, job); err != nil {
			return err
		}
		return s.fail(ctx, unit, domain.FailureLegacyVolume, fmt.Errorf("%w: %s", errLegacy, job.FileName))
	}
	refused, err := s.refusedAsLegacySet(ctx, job)
	if err != nil {
		return err
	}
	if refused {
		return s.fail(ctx, unit, domain.FailureLegacyVolume, fmt.Errorf("%w: %s has old volumes in its batch", errLegacy, job.FileName))
	}

	archive := s.archivePath(job)
	header, err := x.workspace.ReadHeader(archive, 8)
	if err != nil {
		return fmt.Errorf("read upload %s: %w", job.ID, err)
	}
	if _, isArchive := domain.DetectArchive(header); !isArchive {
		if !job.IsVolume() {
			return s.filterUpload(ctx, unit) // not an archive: it is filtered as it was uploaded (RF-07)
		}
		return s.stop(ctx, unit, fmt.Errorf("%w: %s is not an archive", ErrCorrupt, job.FileName))
	}

	if job.Status != domain.StatusExtracting { // a retry or a claimed set is in extracting already
		if err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
			return j.StartExtraction(now)
		}); err != nil {
			return s.abortedOr(ctx, err)
		}
	}

	listing, err := x.archives.List(ctx, archive, r.password)
	if err == nil && listing.Encrypted() && r.password == "" {
		err = ErrPasswordRequired
	}
	if err == nil {
		if bad := unsafeListing(listing); bad != "" {
			err = fmt.Errorf("%w: %q", ErrUnsafeEntry, bad)
		}
	}
	if err != nil {
		return s.stop(ctx, unit, err)
	}

	if free, err := x.workspace.FreeSpace(); err != nil {
		s.log.Warn("free space of the library not measured", "job", job.ID, "error", err)
	} else if need := uint64(max(listing.TotalSize(), 0)) + domain.FreeSpaceMargin; need > free { //nolint:gosec // non-negative, checked above
		return s.stop(ctx, unit, fmt.Errorf("%w: %d bytes needed, %d free", ErrNoSpace, need, free))
	}

	if err := x.workspace.ResetExtraction(job.ID); err != nil {
		return err
	}
	var last time.Time
	warning, err := x.archives.Extract(ctx, archive, x.workspace.ExtractionDir(job.ID), r.password, listing, func(pct int) {
		if now := s.now(); now.Sub(last) >= progressInterval {
			last = now
			s.setProgress(ctx, unit, pct)
		}
	})
	if err != nil {
		return s.stop(ctx, unit, err)
	}
	if err := x.workspace.CheckExtraction(job.ID); err != nil {
		return s.stop(ctx, unit, err)
	}
	if warning != "" {
		s.log.Warn("archive attributes not applied", "job", job.ID, "warning", warning)
	}
	return s.finish(ctx, unit)
}

// archivePath is the file the extraction reads: the staged upload of a single
// archive, or the volume inside its set's folder once the set is claimed.
func (s *Service) archivePath(job *domain.UploadJob) string {
	if job.IsVolume() {
		return s.work.workspace.VolumePath(job.VolumeGroup, job.FileName)
	}
	return s.work.workspace.ArchivePath(job.ID)
}

// originPath is where a dropped file was in the drop: its path in the folder
// dropped in the browser, or its name when it was dropped on its own (RF-30).
func originPath(job *domain.UploadJob) string {
	if job.RelativePath != "" {
		return job.RelativePath
	}
	return job.FileName
}

// leaderOf is the id of the job an extraction runs for: the job itself, or for a
// set the id of its claim (its first volume). Its folder is named after it.
func leaderOf(job *domain.UploadJob) domain.JobID {
	if job.IsVolume() {
		return domain.JobID(job.VolumeGroup)
	}
	return job.ID
}

// finish records the extraction as done, moves the originals to the trash (RF-06) and
// filters the extracted content (RF-07). The state is saved before the originals move: a
// restart that finds an original still in staging of a job past its extraction moves it to
// the trash then (trashLeftovers), so no cut leaves a job waiting for a file that is already
// in the trash. An original that does not move fails the unit with trash_failed (RF-08a): it
// stays in staging, and the filter waits for Reintentar.
func (s *Service) finish(ctx context.Context, unit []*domain.UploadJob) error {
	if err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.FinishExtraction(now)
	}); err != nil {
		return s.abortedOr(ctx, err)
	}
	if err := s.trashOriginals(ctx, unit); err != nil {
		if ctx.Err() != nil {
			return nil // stopped, not failed: the restart moves what is left (trashLeftovers)
		}
		err = s.failTrash(ctx, unit, err)
		s.settle.mark(unit[0].BatchID) // the failed unit ends, which may let its batch be decided (RF-07)
		return s.abortedOr(ctx, err)
	}
	// The extracted content goes on to the filter (RF-07), in the same worker.
	return s.abortedOr(ctx, s.filterUnit(ctx, unit, true))
}

// trashOriginals moves the originals of a unit to the trash (RF-06), one entry per file. An
// original that is no longer in staging is already there, so a repeated call moves nothing
// twice. Every original is tried; the error joins the moves that failed, and each of those
// originals stays in staging, known to its job, for Reintentar (RF-08a). A set's folder goes
// once none of its volumes is left.
func (s *Service) trashOriginals(ctx context.Context, unit []*domain.UploadJob) error {
	x := s.work
	var failed []error
	for _, m := range unit {
		if !x.workspace.FileExists(s.archivePath(m)) {
			continue
		}
		if err := x.trash.Put(ctx, TrashPut{
			Origin: TrashOriginUpload, Reason: TrashReasonExtracted,
			Name: m.FileName, BatchID: string(m.BatchID), RelativePath: originPath(m),
			Source: s.archivePath(m), Size: m.Size,
		}); err != nil {
			failed = append(failed, fmt.Errorf("original %s: %w", m.FileName, err))
			continue
		}
		if !m.IsVolume() {
			if err := s.staging.Remove(ctx, m.ID); err != nil {
				s.log.Warn("upload leftovers not removed", "job", m.ID, "error", err)
			}
		}
	}
	if len(failed) > 0 {
		return errors.Join(failed...)
	}
	if unit[0].IsVolume() {
		if err := x.workspace.RemoveVolumes(unit[0].VolumeGroup); err != nil {
			s.log.Warn("set folder not removed", "group", unit[0].VolumeGroup, "error", err)
		}
	}
	return nil
}

// trashLeftovers moves to the trash the originals that a restart found still in staging,
// of the jobs past their extraction (RF-06). finish saves the state before it moves an
// original, so an interrupted move of a single archive or of a set's volumes is finished
// here, one volume at a time, and the rest of the set is not left in staging. It runs once
// at start, before the library is served.
func (s *Service) trashLeftovers(ctx context.Context) {
	if s.work == nil {
		return
	}
	filtering, err := s.jobs.ByStatus(ctx, domain.StatusFiltering)
	if err != nil {
		s.log.Warn("originals not checked", "error", err)
		return
	}
	seen := map[domain.JobID]bool{}
	for _, job := range filtering {
		leader := leaderOf(job)
		if seen[leader] {
			continue
		}
		seen[leader] = true
		if !s.extracted(job) {
			continue // a file uploaded on its own: it is kept, not extracted
		}
		unit, err := s.unitOf(ctx, job)
		if err != nil {
			s.log.Warn("originals not trashed", "job", job.ID, "error", err)
			continue
		}
		if err := s.trashOriginals(ctx, unit); err != nil && ctx.Err() == nil {
			if ferr := s.failTrash(ctx, unit, err); ferr != nil && ctx.Err() == nil {
				s.log.Warn("trash failure not recorded", "job", job.ID, "error", ferr)
			}
		}
	}
}

// stop ends an extraction that did not succeed. Whatever was extracted is
// deleted. A password problem pauses the job; the archive stays in staging for
// a failure, so that it can be retried (RF-10c).
func (s *Service) stop(ctx context.Context, unit []*domain.UploadJob, cause error) error {
	if ctx.Err() != nil {
		return nil // cancelled: the worker removes what it extracted
	}
	leader := leaderOf(unit[0])
	switch {
	case errors.Is(cause, ErrPasswordRequired):
		s.removeExtraction(leader)
		return s.abortedOr(ctx, s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error { return j.AskPassword(now) }))
	case errors.Is(cause, ErrWrongPassword):
		s.removeExtraction(leader)
		return s.abortedOr(ctx, s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error { return j.RejectPassword(now) }))
	case errors.Is(cause, ErrNoSpace):
		return s.fail(ctx, unit, domain.FailureNoSpace, cause)
	case errors.Is(cause, ErrUnsafeEntry):
		return s.fail(ctx, unit, domain.FailureArchiveUnsafe, cause)
	case errors.Is(cause, ErrMismatch):
		return s.fail(ctx, unit, domain.FailureCRCMismatch, cause)
	case errors.Is(cause, ErrCorrupt):
		return s.fail(ctx, unit, domain.FailureArchiveCorrupt, cause)
	default:
		// Not the archive's fault (the staging area, the database): the job stays as
		// it is and the cause goes back to the worker, which logs it.
		s.removeExtraction(leader)
		return cause
	}
}

// fail ends the job, or every volume of its set, with the reason, and deletes
// what was extracted. The archive is kept when the reason admits a retry
// (RF-10c); otherwise an upload's archive is deleted, and a Samba copy goes to
// the trash (RF-27a).
func (s *Service) fail(ctx context.Context, unit []*domain.UploadJob, reason domain.FailureReason, cause error) error {
	s.removeExtraction(leaderOf(unit[0]))
	s.log.Info("extraction failed", "job", unit[0].ID, "reason", reason, "detail", cause.Error())
	if !reason.RetriesExtraction() {
		s.discardFailed(ctx, unit) // the copy of a Samba file goes to the trash, never deleted (RF-27a)
	}
	err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.Fail(reason, now)
	})
	s.settle.mark(unit[0].BatchID) // the empty jobs of its batch may now be decided (RF-07)
	return s.abortedOr(ctx, err)
}

// removeArchives deletes what is staged of a job, or of its set. It is called only where the
// originals are not waiting for the trash: a failure before the extraction ended (fail), or
// after the originals reached the trash (endEmpty, which trashes them first).
func (s *Service) removeArchives(ctx context.Context, unit []*domain.UploadJob) {
	if unit[0].IsVolume() {
		if err := s.work.workspace.RemoveVolumes(unit[0].VolumeGroup); err != nil {
			s.log.Warn("set folder not removed", "group", unit[0].VolumeGroup, "error", err)
		}
		return
	}
	if err := s.staging.Remove(ctx, unit[0].ID); err != nil {
		s.log.Warn("upload not removed", "job", unit[0].ID, "error", err)
	}
}

// removeExtraction deletes the extracted files of a job, logging a failure.
func (s *Service) removeExtraction(id domain.JobID) {
	if err := s.work.workspace.RemoveExtraction(id); err != nil {
		s.log.Warn("extracted files not removed", "job", id, "error", err)
	}
}

// abortedOr turns the error of a change that a cancellation (or a shutdown)
// overtook into nil: the job was ended on purpose, so there is no failure to report.
func (s *Service) abortedOr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return nil
	}
	return err
}

// update applies fn to a job under the service lock and saves it.
func (s *Service) update(ctx context.Context, id domain.JobID, fn func(*domain.UploadJob, time.Time) error) error {
	_, err := s.mutateChanged(ctx, id, func(j *domain.UploadJob) (bool, error) {
		if err := fn(j, s.now()); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

// applyAll applies fn to every job of the unit, each saved and announced. The
// first error is returned, after the others were tried.
func (s *Service) applyAll(ctx context.Context, unit []*domain.UploadJob, fn func(*domain.UploadJob, time.Time) error) error {
	var first error
	for _, m := range unit {
		if err := s.update(ctx, m.ID, fn); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// setProgress records the percentage extracted on every job of the unit.
func (s *Service) setProgress(ctx context.Context, unit []*domain.UploadJob, pct int) {
	for _, m := range unit {
		if err := s.mutate(ctx, m.ID, func(j *domain.UploadJob) (bool, error) {
			return j.RecordExtractionProgress(pct, s.now()), nil
		}); err != nil {
			s.log.Warn("extraction progress not recorded", "job", m.ID, "error", err)
		}
	}
}

// extractable reports whether a request may extract its job now: an uploaded
// single archive, one waiting for a password that was sent (RF-04), or a job
// that was put in extracting already (a retry, or a claimed set; RF-10c, RF-03).
func extractable(j *domain.UploadJob, r extractionRequest) bool {
	switch j.Status {
	case domain.StatusUploaded:
		return !j.IsVolume()
	case domain.StatusNeedsPassword:
		return r.password != ""
	case domain.StatusExtracting:
		return r.started
	default:
		return false
	}
}

// SubmitPassword sends the password of a job waiting in needs_password (RF-04).
// The job is extracted again in the background; the answer is the job as it is
// now, which is still waiting until a worker takes it. For a set, the password
// goes to the set's extraction.
func (s *Service) SubmitPassword(ctx context.Context, id domain.JobID, password string) (domain.JobView, error) {
	if s.work == nil {
		return domain.JobView{}, ErrExtractionDisabled
	}
	if password == "" || len(password) > maxPasswordBytes || strings.ContainsRune(password, 0) {
		return domain.JobView{}, ErrInvalidPassword
	}
	job, err := s.Job(ctx, id)
	if err != nil {
		return domain.JobView{}, err
	}
	if job.Status != domain.StatusNeedsPassword {
		return domain.JobView{}, ErrNotWaitingForPassword
	}
	s.work.enqueue(extractionRequest{id: leaderOf(job), password: password})
	return s.view(ctx, job)
}

// Retry sends a failed job back to the step that failed (RF-10c, RF-07, RF-08a): an
// extraction that failed for a retryable reason is extracted again from its archive, which
// must still be in staging; a filter failure is filtered again from what was recorded; a move
// failure moves the content again from where it stopped. The jobs of the same move (the batch,
// or the unit of a batch without relation) are sent back together. A volume retries its set.
func (s *Service) Retry(ctx context.Context, id domain.JobID) (domain.JobView, error) {
	if s.work == nil {
		return domain.JobView{}, ErrExtractionDisabled
	}
	job, err := s.Job(ctx, id)
	if err != nil {
		return domain.JobView{}, err
	}
	if job.Status != domain.StatusFailed {
		return domain.JobView{}, ErrCannotRetry
	}
	switch job.Error {
	case domain.FailureFilterFailed:
		return s.retryFiltering(ctx, job, false)
	case domain.FailureMoveFailed:
		return s.retryFiltering(ctx, job, true)
	case domain.FailureTrashFailed:
		return s.retryTrash(ctx, job)
	}
	if !job.Error.RetriesExtraction() {
		return domain.JobView{}, ErrCannotRetry
	}
	return s.retryExtraction(ctx, job)
}

// retryExtraction extracts again a job that failed for a retryable extraction reason (RF-10c).
// Its archive must still be in staging; the files of the failed attempt are gone.
func (s *Service) retryExtraction(ctx context.Context, job *domain.UploadJob) (domain.JobView, error) {
	unit, err := s.unitOf(ctx, job)
	if err != nil {
		return domain.JobView{}, ErrCannotRetry
	}
	if !s.archiveStaged(ctx, job, unit) {
		return domain.JobView{}, ErrCannotRetry
	}
	if err := s.applyAll(ctx, unit, func(j *domain.UploadJob, now time.Time) error {
		return j.RetryExtraction(now)
	}); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return domain.JobView{}, ErrCannotRetry
		}
		return domain.JobView{}, err
	}
	s.work.enqueue(extractionRequest{id: leaderOf(job), started: true})
	return s.reloaded(ctx, job.ID)
}

// retryFiltering sends the jobs of a failed filter or move back to filtering (RF-07, RF-08a).
// A filter failure is filtered again by the worker; a move failure is settled again, and the
// batch moves what is left of its content.
func (s *Service) retryFiltering(ctx context.Context, job *domain.UploadJob, move bool) (domain.JobView, error) {
	group, err := s.retryGroup(ctx, job, move)
	if err != nil {
		return domain.JobView{}, ErrCannotRetry
	}
	if err := s.applyAll(ctx, group, func(j *domain.UploadJob, now time.Time) error {
		return j.RetryFiltering(now)
	}); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return domain.JobView{}, ErrCannotRetry
		}
		return domain.JobView{}, err
	}
	s.work.enqueue(extractionRequest{id: leaderOf(job), refilter: !move, settle: move})
	return s.reloaded(ctx, job.ID)
}

// retryGroup returns the jobs a retry sends back together: the unit of the job, or for a
// move that a batch with a relation made, every job of the batch that failed the same way.
func (s *Service) retryGroup(ctx context.Context, job *domain.UploadJob, move bool) ([]*domain.UploadJob, error) {
	if !move {
		return s.unitOf(ctx, job)
	}
	batch, err := s.batches.Get(ctx, job.BatchID)
	if err != nil {
		return nil, err
	}
	if batch.Relation == domain.RelationNone {
		return s.unitOf(ctx, job)
	}
	jobs, err := s.jobs.ByBatch(ctx, job.BatchID)
	if err != nil {
		return nil, err
	}
	var group []*domain.UploadJob
	for _, j := range jobs {
		if j.Status == domain.StatusFailed && j.Error == domain.FailureMoveFailed {
			group = append(group, j)
		}
	}
	return group, nil
}

// reloaded returns a job as it is now, with its batch.
func (s *Service) reloaded(ctx context.Context, id domain.JobID) (domain.JobView, error) {
	job, err := s.Job(ctx, id)
	if err != nil {
		return domain.JobView{}, err
	}
	return s.view(ctx, job)
}

// refilter filters again a job that failed with filter_failed: the content is filtered from
// what the filter recorded, and the job goes on to the move once its batch is decided (RF-07).
func (s *Service) refilter(ctx context.Context, job *domain.UploadJob) error {
	if job.Status != domain.StatusFiltering || job.FilterDone {
		return nil
	}
	unit, err := s.unitOf(ctx, job)
	if err != nil {
		return err
	}
	extracted := job.IsVolume() || s.work.workspace.ExtractionExists(leaderOf(job))
	return s.filterUnit(ctx, unit, extracted)
}

// archiveStaged reports whether the archive of a failed job, or the set's first
// volume, is still there.
func (s *Service) archiveStaged(ctx context.Context, job *domain.UploadJob, unit []*domain.UploadJob) bool {
	if !job.IsVolume() {
		return s.work.workspace.ArchiveExists(job.ID)
	}
	leader, err := s.Job(ctx, leaderOf(job))
	if err != nil {
		return false
	}
	if len(unit) == 0 {
		return false
	}
	return s.work.workspace.VolumeExists(leader.VolumeGroup, leader.FileName)
}

// abandonExtraction stops the extraction of a job, or of a claimed set (whose
// id is the group's), whose upload the browser deleted: its worker is cancelled
// and its extracted files are removed (RF-09a).
func (s *Service) abandonExtraction(id domain.JobID) {
	if s.work == nil {
		return
	}
	s.work.mu.Lock()
	cancel, ok := s.work.running[id]
	s.work.mu.Unlock()
	if ok {
		cancel()
	}
	s.removeExtraction(id)
}

// unsafeListing returns the first entry that would leave the extraction folder
// (an absolute path or a `..` segment, the zip-slip of RNF-02) or is a link.
func unsafeListing(l Listing) string {
	for _, e := range l.Entries {
		if e.Symlink || unsafePath(e.Path) {
			return e.Path
		}
	}
	return ""
}

func unsafePath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	if path.IsAbs(p) || (len(p) > 1 && p[1] == ':') {
		return true
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// view returns a job with its batch, as the API and the event stream show it.
func (s *Service) view(ctx context.Context, job *domain.UploadJob) (domain.JobView, error) {
	batch, err := s.batches.Get(ctx, job.BatchID)
	if err != nil {
		return domain.JobView{}, fmt.Errorf("batch of upload job %s: %w", job.ID, err)
	}
	return domain.JobView{Job: *job, Batch: *batch}, nil
}
