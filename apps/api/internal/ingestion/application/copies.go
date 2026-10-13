package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// copyOf returns where the copy of a job is now: its volume in the folder of its claimed set, or its
// staged file. It returns "" when no copy is left there.
func (s *Service) copyOf(job *domain.UploadJob) string {
	if job.IsVolume() && job.VolumeGroup != "" && s.work.workspace.VolumeExists(job.VolumeGroup, job.FileName) {
		return s.work.workspace.VolumePath(job.VolumeGroup, job.FileName)
	}
	if s.work.workspace.ArchiveExists(job.ID) {
		return s.work.workspace.ArchivePath(job.ID)
	}
	return ""
}

// discardFailed ends the copies of a unit that failed with a reason Reintentar does not fix. The copy of an
// upload is deleted, as RF-10c says. The copy of a Samba file is the only one left in the library, so it
// goes to the trash instead, and is never deleted (RF-27a).
func (s *Service) discardFailed(ctx context.Context, unit []*domain.UploadJob) {
	if !unit[0].IsSamba() {
		s.removeArchives(ctx, unit)
		return
	}
	if err := s.discardCopies(ctx, unit); err != nil && ctx.Err() == nil {
		s.log.Error("samba copy not sent to the trash; it stays in staging", "job", unit[0].ID, "error", err)
	}
}

// discardCopies sends the copies of a Samba unit to the trash, one entry per file, with origin samba and the
// reason removed (RF-27a, RF-30). A copy whose move fails stays in staging, and the error says which. The
// folder of a claimed set goes once none of its volumes is left.
func (s *Service) discardCopies(ctx context.Context, unit []*domain.UploadJob) error {
	var failed []error
	for _, job := range unit {
		src := s.copyOf(job)
		if src == "" {
			continue
		}
		// A copy of Samba has no batch in the trash (the catalog's rule for origin samba); its place in the
		// drop is the relative path, which is its file name here.
		if err := s.work.trash.Put(ctx, TrashPut{
			Origin: TrashOriginSamba, Reason: TrashReasonRemoved,
			Name: job.FileName, RelativePath: originPath(job),
			Source: src, Size: job.Size,
		}); err != nil {
			failed = append(failed, fmt.Errorf("copy %s: %w", job.FileName, err))
		}
	}
	if len(failed) > 0 {
		return errors.Join(failed...)
	}
	if unit[0].IsVolume() && unit[0].VolumeGroup != "" {
		if err := s.work.workspace.RemoveVolumes(unit[0].VolumeGroup); err != nil {
			s.log.Warn("set folder not removed", "group", unit[0].VolumeGroup, "error", err)
		}
	}
	return nil
}
