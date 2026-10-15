package http

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// PlanTitleIDTag implements httpapi.StrictServerInterface (RF-45): the preview of putting the ID tag on a title or
// taking it off.
func (h *LibraryHandler) PlanTitleIDTag(ctx context.Context, req httpapi.PlanTitleIDTagRequestObject) (httpapi.PlanTitleIDTagResponseObject, error) {
	choice, ok := idTagRequestOf(req.Body)
	if !ok {
		return httpapi.PlanTitleIDTag400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	plan, err := h.edits.PlanIDTag(ctx, req.Id, choice)
	switch {
	case errors.Is(err, application.ErrTitleNotFound):
		return httpapi.PlanTitleIDTag404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.PlanTitleIDTag200JSONResponse(editPlanToAPI(plan)), nil
}

// CommitTitleIDTag implements httpapi.StrictServerInterface (RF-45): it stores the ID tag action of a title as one change.
func (h *LibraryHandler) CommitTitleIDTag(ctx context.Context, req httpapi.CommitTitleIDTagRequestObject) (httpapi.CommitTitleIDTagResponseObject, error) {
	choice, ok := idTagRequestOf(req.Body)
	if !ok {
		return httpapi.CommitTitleIDTag400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(badRequest()),
		}, nil
	}
	res, err := h.edits.CommitIDTag(ctx, req.Id, choice)
	var invalid *application.EditInvalidError
	switch {
	case errors.Is(err, application.ErrTitleNotFound):
		return httpapi.CommitTitleIDTag404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	case errors.Is(err, application.ErrPlanChanged), errors.Is(err, application.ErrTargetTaken):
		if errors.Is(err, application.ErrTargetTaken) {
			h.log.WarnContext(ctx, "ID tag refused: a target was taken", "title", req.Id, "error", err)
		}
		return httpapi.CommitTitleIDTag409ApplicationProblemPlusJSONResponse{
			EditConflictApplicationProblemPlusJSONResponse: editConflict(),
		}, nil
	case errors.As(err, &invalid):
		return httpapi.CommitTitleIDTag422JSONResponse{EditInvalidJSONResponse: httpapi.EditInvalidJSONResponse(editPlanToAPI(invalid.Plan))}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.CommitTitleIDTag200JSONResponse(httpapi.EditResult{
		TitleId: res.TitleID, Folder: res.Folder, Merged: res.Merged, Changed: res.Changed,
	}), nil
}

// idTagRequestOf turns the body of the ID tag action into the application's request. It answers false when the body is missing.
func idTagRequestOf(body *httpapi.IDTagRequest) (application.IDTagRequest, bool) {
	if body == nil {
		return application.IDTagRequest{}, false
	}
	out := application.IDTagRequest{Revision: body.Revision, IDTag: body.IdTag}
	if body.Videos != nil {
		for _, v := range *body.Videos {
			c := application.IDTagVideo{FileID: v.FileId, Version: stringOf(v.Version)}
			if v.Duplicate != nil {
				c.Duplicate = domain.Decision(*v.Duplicate)
			}
			out.Videos = append(out.Videos, c)
		}
	}
	if body.Labels != nil {
		for _, l := range *body.Labels {
			out.Labels = append(out.Labels, application.LabelChoice{Rel: l.Rel, Version: l.Version})
		}
	}
	return out, true
}
