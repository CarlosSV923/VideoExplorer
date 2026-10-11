package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sevenzip"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// discardTrash is the trash of these tests: nothing is moved, nothing is recorded.
type discardTrash struct{}

func (discardTrash) Put(context.Context, application.TrashPut) error { return nil }

// newExtractionRoutes serves the API with the upload endpoint and the extraction
// wired, over a fresh library. The workers do not run: these tests check the
// HTTP contract of the password and retry operations, not the extraction itself.
func newExtractionRoutes(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.DiscardHandler)
	ingest, _ := newTestIngestion(t)
	lib := t.TempDir()
	uploads := newUploads(config.Config{LibraryPath: lib}, log, ingest.Service)
	if uploads.adapter == nil {
		t.Fatal("the test library is not writable")
	}
	ingest.Service.WithStaging(uploads.adapter)
	ingest.Service.WithExtraction(workspace.New(stagingDir(lib), nil), sevenzip.New(""), discardTrash{})
	return newTestRoutesWithUploads(t, false, imagecache.New(t.TempDir(), nil), ingest.Handler, uploads.handler)
}

func TestPasswordAndRetryNeedASession(t *testing.T) {
	t.Parallel()

	h := newExtractionRoutes(t)
	tests := []struct {
		name, path, body string
	}{
		{name: "password", path: "/api/jobs/job1/password", body: `{"password":"x"}`},
		{name: "retry", path: "/api/jobs/job1/retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, h, http.MethodPost, tt.path, tt.body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestPasswordAndRetryAnswerForJobsThatDoNotExistOrCannotTakeThem(t *testing.T) {
	t.Parallel()

	h := newExtractionRoutes(t)
	cookie := loginCookie(t, h)
	batchID := newBatchID(t, h, cookie)
	if rec := tusCreateWith(t, h, "filename "+b64("Limbo.mkv")+",batchId "+b64(batchID), cookie); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}
	uploading := jobIDOf(t, h, cookie, "Limbo.mkv")

	tests := []struct {
		name       string
		path, body string
		wantStatus int
	}{
		{name: "password for an unknown job", path: "/api/jobs/nope/password", body: `{"password":"x"}`, wantStatus: http.StatusNotFound},
		{name: "retry of an unknown job", path: "/api/jobs/nope/retry", wantStatus: http.StatusNotFound},
		{name: "password for a job still uploading", path: "/api/jobs/" + uploading + "/password", body: `{"password":"x"}`, wantStatus: http.StatusConflict},
		{name: "retry of a job still uploading", path: "/api/jobs/" + uploading + "/retry", wantStatus: http.StatusConflict},
		{name: "password without a body", path: "/api/jobs/" + uploading + "/password", wantStatus: http.StatusBadRequest},
		{name: "empty password", path: "/api/jobs/" + uploading + "/password", body: `{"password":""}`, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, h, http.MethodPost, tt.path, tt.body, cookie)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want the RFC 9457 problem", ct)
			}
		})
	}
}

func TestJobsCarryTheProgressOfTheirExtraction(t *testing.T) {
	t.Parallel()

	h := newExtractionRoutes(t)
	cookie := loginCookie(t, h)
	batchID := newBatchID(t, h, cookie)
	if rec := tusCreateWith(t, h, "filename "+b64("Movie.zip")+",batchId "+b64(batchID), cookie); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}
	rec := do(t, h, http.MethodGet, "/api/jobs", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/jobs = %d", rec.Code)
	}
	var jobs []struct {
		Progress *int `json:"progress"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Progress == nil || *jobs[0].Progress != 0 {
		t.Fatalf("jobs = %s, want one job with progress 0", rec.Body.String())
	}
}

// jobIDOf returns the id of the job of the file with that name, from GET /api/jobs.
func jobIDOf(t *testing.T, h http.Handler, cookie *http.Cookie, fileName string) string {
	t.Helper()

	rec := do(t, h, http.MethodGet, "/api/jobs", "", cookie)
	var jobs []jobJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.FileName == fileName {
			return j.ID
		}
	}
	t.Fatalf("no job for %s in %s", fileName, rec.Body.String())
	return ""
}

func TestAVolumeNeedsItsSetSizeAndAnyOtherFileRefusesIt(t *testing.T) {
	t.Parallel()

	h := newExtractionRoutes(t)
	cookie := loginCookie(t, h)
	batchID := newBatchID(t, h, cookie)
	batch := ",batchId " + b64(batchID)
	tests := []struct {
		name       string
		metadata   string
		wantStatus int
	}{
		{name: "volume with its set size", metadata: "filename " + b64("Movie.7z.001") + batch + ",partsTotal " + b64("3"), wantStatus: http.StatusCreated},
		{name: "volume without the set size", metadata: "filename " + b64("Movie.7z.002") + batch, wantStatus: http.StatusBadRequest},
		{name: "set size on a plain file", metadata: "filename " + b64("Movie.mkv") + batch + ",partsTotal " + b64("2"), wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := tusCreateWith(t, h, tt.metadata, cookie)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
