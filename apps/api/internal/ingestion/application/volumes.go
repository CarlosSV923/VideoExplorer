package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// errLegacy is the cause of a legacy_volume failure (RF-03), for the log.
var errLegacy = errors.New("legacy RAR volume is not supported")

// unitOf returns the jobs an extraction works on: the job itself, or for a volume
// every volume of its claimed set, in volume order (RF-03).
func (s *Service) unitOf(ctx context.Context, job *domain.UploadJob) ([]*domain.UploadJob, error) {
	if !job.IsVolume() {
		return []*domain.UploadJob{job}, nil
	}
	if job.VolumeGroup == "" {
		return nil, fmt.Errorf("ingestion: volume %s is not part of a claimed set", job.ID)
	}
	members, err := s.jobs.ByVolumeGroup(ctx, job.VolumeGroup)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("ingestion: set %s has no volumes", job.VolumeGroup)
	}
	return members, nil
}

// afterUpload carries on an upload that just completed (RF-10a): a volume waits
// for the rest of its set, and any other file goes to extraction (RF-04).
func (s *Service) afterUpload(ctx context.Context, id domain.JobID) error {
	job, err := s.Job(ctx, id)
	if err != nil {
		return err
	}
	if !job.IsVolume() {
		s.enqueueJob(id)
		return nil
	}
	if _, err := s.mutateChanged(ctx, id, func(j *domain.UploadJob) (bool, error) {
		if j.Status != domain.StatusUploaded {
			return false, nil
		}
		return true, j.WaitForParts(s.now())
	}); err != nil {
		return err
	}
	return s.checkSet(ctx, id)
}

// checkSet looks at the set of a waiting volume (RF-03). Once every volume the
// set declares has arrived, it checks that they form one set: the same total,
// and positions 1 to total. A set that does not is refused, all its volumes
// fail with parts_mismatch. One that does is claimed: its volumes move into the
// set's folder, and it is extracted from its first volume.
func (s *Service) checkSet(ctx context.Context, id domain.JobID) error {
	if s.work == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return err
	}
	// The set may have ended jobs of its batch (RF-07): the batch is asked for, without waiting.
	defer s.settle.mark(job.BatchID)
	if job.Status != domain.StatusWaitingParts || !job.IsVolume() {
		return nil
	}
	waiting, err := s.waitingSet(ctx, job)
	if err != nil {
		return err
	}
	// Volumes that declare different totals cannot be one set: refused at once.
	if slices.ContainsFunc(waiting, func(v *domain.UploadJob) bool { return v.PartsTotal != job.PartsTotal }) {
		return s.refuseSet(ctx, waiting)
	}
	if len(waiting) < job.PartsTotal {
		return nil // the rest of the set has not arrived yet
	}
	if !formsSet(waiting, job.PartsTotal) {
		if job.IsSamba() {
			// A copy of Samba has the volumes that were copied, and its total is the number found: a
			// hole among them is a missing part, not a set that does not match (RF-27a).
			reason := domain.FailurePartsMissing
			if hasRepeatedVolume(waiting) {
				reason = domain.FailurePartsMismatch
			}
			return s.refuseSetAs(ctx, waiting, reason)
		}
		return s.refuseSet(ctx, waiting)
	}

	first := waiting[0]
	group := string(first.ID)
	for _, v := range waiting {
		if err := s.work.workspace.ClaimVolume(v.ID, group, v.FileName); err != nil {
			return fmt.Errorf("claim volume %s: %w", v.ID, err)
		}
		if err := s.staging.Remove(ctx, v.ID); err != nil {
			s.log.Warn("upload leftovers not removed", "job", v.ID, "error", err)
		}
		if err := v.ClaimSet(group, s.now()); err != nil {
			return err
		}
		if err := s.saveAnnounce(ctx, v); err != nil {
			return err
		}
	}
	s.work.enqueue(extractionRequest{id: first.ID, started: true})
	return nil
}

// refuseSet ends the waiting volumes of a set that does not form one archive
// (RF-10c, parts_mismatch) and deletes their files.
func (s *Service) refuseSet(ctx context.Context, waiting []*domain.UploadJob) error {
	return s.refuseSetAs(ctx, waiting, domain.FailurePartsMismatch)
}

// refuseSetAs ends the waiting volumes of a set with the reason, and deletes their files.
func (s *Service) refuseSetAs(ctx context.Context, waiting []*domain.UploadJob, reason domain.FailureReason) error {
	samba := len(waiting) > 0 && waiting[0].IsSamba()
	if samba {
		// The volumes of a Samba set go to the trash before the set is marked failed, so that a restart
		// between the two never leaves a failed set with its copies still in staging (RF-27a).
		if err := s.discardCopies(ctx, waiting); err != nil && ctx.Err() == nil {
			s.log.Error("samba volumes not sent to the trash; they stay in staging", "set", waiting[0].VolumeSet, "error", err)
		}
	}
	for _, v := range waiting {
		if err := v.Fail(reason, s.now()); err != nil {
			return err
		}
		if err := s.saveAnnounce(ctx, v); err != nil {
			return err
		}
		if !samba {
			if err := s.staging.Remove(ctx, v.ID); err != nil {
				s.log.Warn("volume not removed", "job", v.ID, "error", err)
			}
		}
	}
	return nil
}

// waitingSet returns the volumes of the job's set that are waiting, in volume
// order. Volumes already claimed by an extraction are not part of it.
func (s *Service) waitingSet(ctx context.Context, job *domain.UploadJob) ([]*domain.UploadJob, error) {
	waiting, err := s.jobs.ByStatus(ctx, domain.StatusWaitingParts)
	if err != nil {
		return nil, err
	}
	var set []*domain.UploadJob
	for _, v := range waiting {
		if v.BatchID == job.BatchID && v.VolumeSet == job.VolumeSet {
			set = append(set, v)
		}
	}
	slices.SortFunc(set, func(a, b *domain.UploadJob) int { return a.VolumeIndex - b.VolumeIndex })
	return set, nil
}

// hasRepeatedVolume reports whether two of the waiting volumes have the same position.
func hasRepeatedVolume(waiting []*domain.UploadJob) bool {
	seen := map[int]bool{}
	for _, v := range waiting {
		if seen[v.VolumeIndex] {
			return true
		}
		seen[v.VolumeIndex] = true
	}
	return false
}

// formsSet reports whether the waiting volumes are the whole of a set of total
// volumes: each declares the same total, and their positions are 1 to total.
func formsSet(waiting []*domain.UploadJob, total int) bool {
	if len(waiting) != total {
		return false
	}
	for i, v := range waiting {
		if v.PartsTotal != total || v.VolumeIndex != i+1 {
			return false
		}
	}
	return true
}

// saveAnnounce stores a job changed under the service lock and announces it.
func (s *Service) saveAnnounce(ctx context.Context, job *domain.UploadJob) error {
	job.UpdatedAt = s.now()
	if err := s.jobs.Save(ctx, job); err != nil {
		return fmt.Errorf("save upload job %s: %w", job.ID, err)
	}
	return s.announce(ctx, job)
}

// cancelSet cancels the volumes of a claimed set other than the one the browser
// deleted, and stops the set's extraction (RF-09a): a set is one work.
func (s *Service) cancelSet(ctx context.Context, group string, except domain.JobID) {
	members, err := s.jobs.ByVolumeGroup(ctx, group)
	if err != nil {
		s.log.Warn("set not cancelled", "group", group, "error", err)
		return
	}
	for _, m := range members {
		if s.awaitingTrash(m) {
			// A volume whose move to the trash is pending is not deleted with its set (RF-08a).
			s.log.Error("set kept: a volume waits for its move to the trash", "group", group, "job", m.ID)
			return
		}
	}
	for _, m := range members {
		if m.ID == except || m.Status.Terminal() {
			continue
		}
		if _, err := s.mutateChanged(ctx, m.ID, func(j *domain.UploadJob) (bool, error) {
			if j.Status.Terminal() {
				return false, nil
			}
			return true, j.Cancel(s.now())
		}); err != nil {
			s.log.Warn("volume not cancelled", "job", m.ID, "error", err)
		}
	}
	s.abandonExtraction(domain.JobID(group))
	if err := s.work.workspace.RemoveVolumes(group); err != nil {
		s.log.Warn("set folder not removed", "group", group, "error", err)
	}
}

// partsMissing ends the waiting volumes that did not see their set complete within
// AbandonedAfter, and deletes their files (RF-10c). It returns how many it ended.
func (s *Service) partsMissing(ctx context.Context, cutoff time.Time) (int, error) {
	stale, err := s.jobs.Stale(ctx, domain.StatusWaitingParts, cutoff)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, job := range stale {
		changed, err := s.mutateChanged(ctx, job.ID, func(j *domain.UploadJob) (bool, error) {
			if j.Status != domain.StatusWaitingParts || !j.UpdatedAt.Before(cutoff) {
				return false, nil
			}
			if j.IsSamba() {
				// The copy of a Samba volume goes to the trash, never deleted (RF-27a).
				if err := s.discardCopies(ctx, []*domain.UploadJob{j}); err != nil && ctx.Err() == nil {
					s.log.Error("samba volume not sent to the trash; it stays in staging", "job", j.ID, "error", err)
				}
			} else if err := s.staging.Remove(ctx, j.ID); err != nil {
				return false, fmt.Errorf("remove volume %s: %w", j.ID, err)
			}
			return true, j.Fail(domain.FailurePartsMissing, s.now())
		})
		if err != nil {
			s.log.Warn("volume of a set not purged", "job", job.ID, "error", err)
			continue
		}
		if changed {
			n++
		}
	}
	return n, nil
}
