package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// SubmitJobPassword implements httpapi.StrictServerInterface (RF-04). The
// password is queued for the extraction and never stored or logged.
func (h *Handler) SubmitJobPassword(ctx context.Context, req httpapi.SubmitJobPasswordRequestObject) (httpapi.SubmitJobPasswordResponseObject, error) {
	if req.Body == nil {
		return httpapi.SubmitJobPassword400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The request needs a JSON body with the password.")),
		}, nil
	}
	view, err := h.svc.SubmitPassword(ctx, domain.JobID(req.Id), req.Body.Password)
	switch {
	case errors.Is(err, domain.ErrJobNotFound):
		return httpapi.SubmitJobPassword404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(http.StatusNotFound, "Not Found", "No upload has that id.")),
		}, nil
	case errors.Is(err, application.ErrNotWaitingForPassword):
		return httpapi.SubmitJobPassword409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: httpapi.ConflictApplicationProblemPlusJSONResponse(
				problem(http.StatusConflict, "Conflict", "The upload is not waiting for a password.")),
		}, nil
	case errors.Is(err, application.ErrInvalidPassword):
		return httpapi.SubmitJobPassword400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: httpapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Bad Request", "The password must be 1 to 1024 bytes and have no NUL character.")),
		}, nil
	case errors.Is(err, application.ErrExtractionDisabled):
		return httpapi.SubmitJobPassword409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: httpapi.ConflictApplicationProblemPlusJSONResponse(
				problem(http.StatusConflict, "Conflict", "Archives cannot be extracted: the library is not writable.")),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.SubmitJobPassword202JSONResponse(JobToAPI(view)), nil
}

// RetryJob implements httpapi.StrictServerInterface (RF-10c). Only a failure
// with a retryable reason, whose archive is still in staging, can be retried.
func (h *Handler) RetryJob(ctx context.Context, req httpapi.RetryJobRequestObject) (httpapi.RetryJobResponseObject, error) {
	view, err := h.svc.Retry(ctx, domain.JobID(req.Id))
	switch {
	case errors.Is(err, domain.ErrJobNotFound):
		return httpapi.RetryJob404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(http.StatusNotFound, "Not Found", "No upload has that id.")),
		}, nil
	case errors.Is(err, application.ErrCannotRetry):
		return httpapi.RetryJob409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: httpapi.ConflictApplicationProblemPlusJSONResponse(
				problem(http.StatusConflict, "Conflict", "The upload cannot be retried: it did not fail for a retryable reason, or its archive is gone.")),
		}, nil
	case errors.Is(err, application.ErrExtractionDisabled):
		return httpapi.RetryJob409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: httpapi.ConflictApplicationProblemPlusJSONResponse(
				problem(http.StatusConflict, "Conflict", "Archives cannot be extracted: the library is not writable.")),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.RetryJob202JSONResponse(JobToAPI(view)), nil
}

// DismissJob implements httpapi.StrictServerInterface (RF-27a, "Quitar"): a failed copy of Samba that a retry
// cannot fix leaves No asignados. Any other job answers 409.
func (h *Handler) DismissJob(ctx context.Context, req httpapi.DismissJobRequestObject) (httpapi.DismissJobResponseObject, error) {
	view, err := h.svc.DismissSamba(ctx, domain.JobID(req.Id))
	switch {
	case errors.Is(err, domain.ErrJobNotFound):
		return httpapi.DismissJob404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: httpapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(http.StatusNotFound, "Not Found", "No upload has that id.")),
		}, nil
	case errors.Is(err, domain.ErrNotDismissable):
		return httpapi.DismissJob409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: httpapi.ConflictApplicationProblemPlusJSONResponse(
				problem(http.StatusConflict, "Conflict", "The job cannot be taken off: it is not a failed copy of Samba that a retry cannot fix.")),
		}, nil
	case err != nil:
		return nil, err
	}
	return httpapi.DismissJob200JSONResponse(JobToAPI(view)), nil
}
