package main

import (
	"context"
	"errors"
	"fmt"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	ingestiondomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	ingestionhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// sambaOf gives the ingestion the copies of Samba in No asignados through its port (RF-27a): the
// catalog's types stay out of ingestion.
type sambaOf struct {
	section *catalogapplication.Section
}

var _ ingestionapplication.SambaSection = sambaOf{}

// Files implements ingestionapplication.SambaSection.
func (s sambaOf) Files(ctx context.Context) ([]ingestiondomain.SambaFile, error) {
	files, err := s.section.SambaFiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ingestiondomain.SambaFile, 0, len(files))
	for _, f := range files {
		out = append(out, ingestiondomain.SambaFile{
			Entry: f.Entry, Rel: f.Rel, Name: f.Name, Size: f.Size, Source: f.Source, Copying: f.Copying,
		})
	}
	return out, nil
}

// Take implements ingestionapplication.SambaSection. An entry that changed under the change (copying,
// held by an assignment, or without the file) answers ErrSambaChanged: the next reading takes it again.
func (s sambaOf) Take(ctx context.Context, entry int64, moves []ingestionapplication.SambaMove) error {
	mv := make([]catalogapplication.SambaMove, 0, len(moves))
	for _, m := range moves {
		mv = append(mv, catalogapplication.SambaMove{Rel: m.Rel, Dst: m.Dst})
	}
	err := s.section.TakeSamba(ctx, entry, mv)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, catalogapplication.ErrCopying), errors.Is(err, catalogapplication.ErrAssigning),
		errors.Is(err, catalogapplication.ErrFileNotFound), errors.Is(err, catalogapplication.ErrEntryNotFound):
		return fmt.Errorf("%w: %w", ingestionapplication.ErrSambaChanged, err)
	default:
		return err
	}
}

// arrivalsOf gives No asignados the copies of Samba that the ingestion takes as uploads (RF-27a), and
// the jobs they became, in the contract's shape.
type arrivalsOf struct {
	ingest *ingestionapplication.Service
}

// Adopt implements the section's arrivals: it takes the settled copies of Samba.
func (a arrivalsOf) Adopt(ctx context.Context) error {
	_, err := a.ingest.AdoptSamba(ctx)
	return err
}

// Jobs implements the section's arrivals: the jobs that the section shows, as GET /jobs shows them.
func (a arrivalsOf) Jobs(ctx context.Context) ([]httpapi.UploadJob, error) {
	views, err := a.ingest.SambaJobs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.UploadJob, 0, len(views))
	for _, v := range views {
		out = append(out, ingestionhttp.JobToAPI(v))
	}
	return out, nil
}
