package application

import (
	"context"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// placeBatch moves what a complete batch kept into No asignados (spec RF-08, RF-08a): one
// entry for a batch with a relation, one per unit without relation (domain.PlanEntries). Each
// entry is moved on its own, so a failure of one does not hold back the others.
func (s *Service) placeBatch(ctx context.Context, batch *domain.Batch, kept []*domain.UploadJob) error {
	units, err := s.unitsOf(ctx, kept)
	if err != nil {
		return err
	}
	byLeader := make(map[domain.JobID]domain.UnitContent, len(units))
	for _, u := range units {
		byLeader[u.Leader] = u
	}
	for _, plan := range domain.PlanEntries(*batch, units) {
		s.placeEntry(ctx, batch, plan, byLeader)
	}
	return nil
}

// unitsOf returns the work units of the kept jobs, each once, with what the filter kept of
// it. A set of volumes is one unit, whatever the number of its jobs.
func (s *Service) unitsOf(ctx context.Context, kept []*domain.UploadJob) ([]domain.UnitContent, error) {
	var units []domain.UnitContent
	seen := map[domain.JobID]bool{}
	for _, job := range kept {
		leader := leaderOf(job)
		if seen[leader] {
			continue
		}
		seen[leader] = true
		members, err := s.unitOf(ctx, job)
		if err != nil {
			return nil, err
		}
		entries, err := s.filter.entries.Entries(ctx, leader)
		if err != nil {
			return nil, err
		}
		extracted := job.IsVolume() || (s.work != nil && s.work.workspace.ExtractionExists(leader))
		units = append(units, domain.UnitContent{Leader: leader, Jobs: members, Extracted: extracted, Kept: entries})
	}
	return units, nil
}

// placeEntry moves one entry into No asignados (RF-08a). On success its jobs are done, what
// was staged for them is removed, and its videos are read again at their new place (RF-24).
// On an error of the library they fail with move_failed, which Reintentar moves again; a
// stopped service leaves them as they are, and the next start moves them.
func (s *Service) placeEntry(ctx context.Context, batch *domain.Batch, plan domain.EntryPlan, byLeader map[domain.JobID]domain.UnitContent) {
	var jobs []*domain.UploadJob
	var units []domain.UnitContent
	inEntry := map[domain.JobID]bool{}
	files := make([]AsideFile, 0, len(plan.Files))
	for _, f := range plan.Files {
		u := byLeader[f.Unit]
		if !inEntry[f.Unit] {
			inEntry[f.Unit] = true
			jobs = append(jobs, u.Jobs...)
			units = append(units, u)
		}
		files = append(files, AsideFile{
			Source: s.stagedPath(u, f), Rel: f.Rel, Size: f.Size, Video: f.Kind == domain.KindVideo,
		})
	}

	places, err := s.library.PutAside(ctx, AsideRequest{
		Source: plan.Key, Name: plan.Name, Loose: plan.Loose,
		BatchID: string(batch.ID), Relation: string(batch.Relation), Files: files,
	})
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("content not moved to unassigned", "batch", batch.ID, "entry", plan.Name, "error", err)
			s.failMove(ctx, jobs)
		}
		return
	}
	if err := s.applyAll(ctx, jobs, func(j *domain.UploadJob, now time.Time) error { return j.Done(now) }); err != nil {
		if ctx.Err() == nil {
			s.log.Warn("moved job not done", "batch", batch.ID, "error", err)
		}
		return
	}

	for _, u := range units {
		if u.Extracted {
			s.removeExtraction(u.Leader) // what is left of the extraction: the discarded files are in the trash
		} else if err := s.staging.Remove(ctx, u.Jobs[0].ID); err != nil {
			s.log.Warn("staged file not removed", "job", u.Jobs[0].ID, "error", err)
		}
	}
	for i, f := range plan.Files {
		if f.Kind == domain.KindVideo {
			s.queueFinal(byLeader[f.Unit].Jobs[0].ID, places[files[i].Source])
		}
	}
	s.log.Info("content moved to unassigned", "batch", batch.ID, "entry", plan.Name, "files", len(files))
}

// stagedPath is where a planned file is now: in its unit's extraction folder, or the staged
// file of a job uploaded on its own.
func (s *Service) stagedPath(u domain.UnitContent, f domain.PlannedFile) string {
	if u.Extracted {
		return s.work.workspace.ExtractedPath(u.Leader, f.Path)
	}
	return s.work.workspace.ArchivePath(u.Jobs[0].ID)
}

// failMove ends jobs whose content did not reach No asignados, with move_failed (RF-08a).
func (s *Service) failMove(ctx context.Context, jobs []*domain.UploadJob) {
	if err := s.applyAll(ctx, jobs, func(j *domain.UploadJob, now time.Time) error {
		return j.Fail(domain.FailureMoveFailed, now)
	}); err != nil && ctx.Err() == nil {
		s.log.Warn("move failure not recorded", "error", err)
	}
}
