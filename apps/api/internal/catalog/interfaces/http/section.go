package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// SectionArrivals is what the section shows of the copies of Samba that the ingestion takes as uploads
// (RF-27a). Reading the section takes the settled copies first, and lists the jobs they became.
type SectionArrivals interface {
	// Adopt takes the settled copies of Samba. A failure is logged by the caller, and the section is read anyway.
	Adopt(ctx context.Context) error
	// Jobs returns the jobs the section shows for the copies of Samba, as GET /jobs shows them.
	Jobs(ctx context.Context) ([]httpapi.UploadJob, error)
}

// noArrivals is the arrivals of a section without any ingestion: nothing is taken and nothing is shown.
type noArrivals struct{}

func (noArrivals) Adopt(context.Context) error { return nil }

func (noArrivals) Jobs(context.Context) ([]httpapi.UploadJob, error) { return nil, nil }

// SectionHandler implements the operations of No asignados (RF-27) of httpapi.StrictServerInterface:
// the section as the disk holds it, and the changes of its entries.
type SectionHandler struct {
	svc      *application.Section
	arrivals SectionArrivals
	log      *slog.Logger
}

// NewSectionHandler builds the handler over the section. arrivals may be nil, and log too.
func NewSectionHandler(svc *application.Section, arrivals SectionArrivals, log *slog.Logger) *SectionHandler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if arrivals == nil {
		arrivals = noArrivals{}
	}
	return &SectionHandler{svc: svc, arrivals: arrivals, log: log}
}

// GetUnassigned implements httpapi.StrictServerInterface (RF-27, RF-27a): it takes the settled copies of
// Samba, then reads the section now and lists the jobs of those copies. A copy that cannot be taken yet
// does not fail the read.
func (h *SectionHandler) GetUnassigned(ctx context.Context, _ httpapi.GetUnassignedRequestObject) (httpapi.GetUnassignedResponseObject, error) {
	if err := h.arrivals.Adopt(ctx); err != nil {
		h.log.WarnContext(ctx, "copies of Samba not taken", "error", err)
	}
	entries, err := h.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	jobs, err := h.arrivals.Jobs(ctx)
	if err != nil {
		return nil, err
	}
	out := sectionToAPI(entries)
	out.Jobs = jobs
	if out.Jobs == nil {
		out.Jobs = []httpapi.UploadJob{}
	}
	return httpapi.GetUnassigned200JSONResponse(out), nil
}

// TrashUnassignedEntry implements httpapi.StrictServerInterface (RF-27, RF-30): the entry goes to the trash.
func (h *SectionHandler) TrashUnassignedEntry(ctx context.Context, req httpapi.TrashUnassignedEntryRequestObject) (httpapi.TrashUnassignedEntryResponseObject, error) {
	err := h.svc.TrashEntry(ctx, req.Id)
	switch {
	case err == nil:
		return httpapi.TrashUnassignedEntry204Response{}, nil
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.TrashUnassignedEntry404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case isEntryConflict(err):
		return httpapi.TrashUnassignedEntry409ApplicationProblemPlusJSONResponse{
			EntryConflictApplicationProblemPlusJSONResponse: entryConflict(err),
		}, nil
	default:
		return nil, err
	}
}

// DeleteUnassignedEntry implements httpapi.StrictServerInterface (RF-27, RF-30): the entry is deleted for good.
func (h *SectionHandler) DeleteUnassignedEntry(ctx context.Context, req httpapi.DeleteUnassignedEntryRequestObject) (httpapi.DeleteUnassignedEntryResponseObject, error) {
	err := h.svc.DeleteEntry(ctx, req.Id)
	switch {
	case err == nil:
		return httpapi.DeleteUnassignedEntry204Response{}, nil
	case errors.Is(err, application.ErrEntryNotFound):
		return httpapi.DeleteUnassignedEntry404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case isEntryConflict(err):
		return httpapi.DeleteUnassignedEntry409ApplicationProblemPlusJSONResponse{
			EntryConflictApplicationProblemPlusJSONResponse: entryConflict(err),
		}, nil
	default:
		return nil, err
	}
}

// TrashUnassignedFile implements httpapi.StrictServerInterface (RF-27, RF-30): one file goes to the trash.
func (h *SectionHandler) TrashUnassignedFile(ctx context.Context, req httpapi.TrashUnassignedFileRequestObject) (httpapi.TrashUnassignedFileResponseObject, error) {
	if req.Body == nil {
		return httpapi.TrashUnassignedFile400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	err := h.svc.TrashFile(ctx, req.Id, req.Body.Path)
	switch {
	case err == nil:
		return httpapi.TrashUnassignedFile204Response{}, nil
	case errors.Is(err, application.ErrEntryNotFound), errors.Is(err, application.ErrFileNotFound):
		return httpapi.TrashUnassignedFile404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case errors.Is(err, application.ErrInvalidPath):
		return httpapi.TrashUnassignedFile400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	case isEntryConflict(err):
		return httpapi.TrashUnassignedFile409ApplicationProblemPlusJSONResponse{
			EntryConflictApplicationProblemPlusJSONResponse: entryConflict(err),
		}, nil
	default:
		return nil, err
	}
}

// DeleteUnassignedFile implements httpapi.StrictServerInterface (RF-27, RF-30): one file is deleted for good.
func (h *SectionHandler) DeleteUnassignedFile(ctx context.Context, req httpapi.DeleteUnassignedFileRequestObject) (httpapi.DeleteUnassignedFileResponseObject, error) {
	if req.Body == nil {
		return httpapi.DeleteUnassignedFile400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	err := h.svc.DeleteFile(ctx, req.Id, req.Body.Path)
	switch {
	case err == nil:
		return httpapi.DeleteUnassignedFile204Response{}, nil
	case errors.Is(err, application.ErrEntryNotFound), errors.Is(err, application.ErrFileNotFound):
		return httpapi.DeleteUnassignedFile404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case errors.Is(err, application.ErrInvalidPath):
		return httpapi.DeleteUnassignedFile400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	case isEntryConflict(err):
		return httpapi.DeleteUnassignedFile409ApplicationProblemPlusJSONResponse{
			EntryConflictApplicationProblemPlusJSONResponse: entryConflict(err),
		}, nil
	default:
		return nil, err
	}
}

// isEntryConflict reports whether err says that the entry cannot change now (RF-26, RF-27).
func isEntryConflict(err error) bool {
	return errors.Is(err, application.ErrAssigning) || errors.Is(err, application.ErrCopying)
}

// entryConflict is the 409 of a change that the entry does not allow, with its stable code.
func entryConflict(err error) httpapi.EntryConflictApplicationProblemPlusJSONResponse {
	code, detail := httpapi.EntryProblemCode("copying"), "A file of the entry is still being copied. Nothing was changed."
	if errors.Is(err, application.ErrAssigning) {
		code, detail = httpapi.EntryProblemCode("assigning"), "The entry is being assigned in another tab. Nothing was changed."
	}
	return httpapi.EntryConflictApplicationProblemPlusJSONResponse(httpapi.EntryProblem{
		Code: code, Status: http.StatusConflict, Title: "Conflict", Detail: &detail,
	})
}

// sectionToAPI turns the entries of the section into the contract's shape. Lists are never null.
func sectionToAPI(entries []application.SectionEntry) httpapi.UnassignedSection {
	out := httpapi.UnassignedSection{Entries: make([]httpapi.UnassignedEntry, 0, len(entries))}
	for _, e := range entries {
		v := httpapi.UnassignedEntry{
			Id:       e.ID,
			Name:     e.Name,
			Origin:   httpapi.UnassignedEntryOrigin(e.Origin),
			Relation: httpapi.BatchRelation(e.Relation),
			Loose:    e.Loose,
			Status:   httpapi.UnassignedEntryStatus(e.Status),
			Size:     e.Size,
			Files:    make([]httpapi.UnassignedFile, 0, len(e.Files)),
		}
		for _, f := range e.Files {
			v.Files = append(v.Files, httpapi.UnassignedFile{Path: f.Path, Size: f.Size, Copying: f.Copying})
		}
		out.Entries = append(out.Entries, v)
	}
	return out
}
