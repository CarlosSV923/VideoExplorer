package application

import (
	"context"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// refusedAsLegacySet reports whether a plain .rar belongs to an old-style RAR set
// whose volumes (movie.r00, movie.r01…) are in the same batch. Such a set is not
// supported, so the .rar fails with legacy_volume too (RF-03). The volumes count
// whatever their state: they are part of the batch once admitted.
func (s *Service) refusedAsLegacySet(ctx context.Context, job *domain.UploadJob) (bool, error) {
	base, ok := domain.PlainRARBase(job.FileName)
	if !ok || job.IsVolume() {
		return false, nil
	}
	members, err := s.jobs.ByBatch(ctx, job.BatchID)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		if other, ok := domain.LegacyVolumeBase(m.FileName); ok && other == base {
			return true, nil
		}
	}
	return false, nil
}

// failRARSiblings ends the plain .rar of the batch that belongs to the set of an
// old-style volume (RF-03), whatever its state: a .rar still arriving, waiting or
// being extracted. Its extraction, if any, is stopped and its files are removed.
// A failure to end one .rar is logged and the others go on.
func (s *Service) failRARSiblings(ctx context.Context, volume *domain.UploadJob) error {
	base, ok := domain.LegacyVolumeBase(volume.FileName)
	if !ok {
		return nil
	}
	members, err := s.jobs.ByBatch(ctx, volume.BatchID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.Status.Terminal() || m.IsVolume() {
			continue
		}
		if other, ok := domain.PlainRARBase(m.FileName); !ok || other != base {
			continue
		}
		s.abandonExtraction(m.ID)
		unit := []*domain.UploadJob{m}
		if err := s.fail(ctx, unit, domain.FailureLegacyVolume, fmt.Errorf("%w: %s", errLegacy, m.FileName)); err != nil {
			s.log.Warn("rar of an old set not ended", "job", m.ID, "error", err)
		}
	}
	return nil
}
