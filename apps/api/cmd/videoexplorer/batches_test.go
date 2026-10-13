package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// newTestIngestion wires the ingestion context over a fresh database and
// returns the database too, to read the jobs back as the tests need.
func newTestIngestion(t *testing.T) (ingestionWiring, *sql.DB) {
	t.Helper()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return newIngestion(conn, slog.New(slog.DiscardHandler), nil), conn
}

// createBatch posts a batch and returns the response.
func createBatch(t *testing.T, h http.Handler, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h, http.MethodPost, "/api/batches", body, cookie)
}

// tusCreateWith starts an upload of 10 bytes with the given Upload-Metadata.
func tusCreateWith(t *testing.T, h http.Handler, uploadMetadata string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, uploadsPath, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", "10")
	req.Header.Set("Upload-Metadata", uploadMetadata)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// tusWrite sends the whole content of an upload, from offset 0, and returns the response.
func tusWrite(t *testing.T, h http.Handler, location, content string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	id := location[strings.LastIndex(location, "/")+1:]
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, uploadsPath+id, strings.NewReader(content))
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Offset", "0")
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// loginCookie signs in with the test password and returns the session cookie.
func loginCookie(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()

	login := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	cookie := sessionCookie(login)
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}
	return cookie
}

func TestBatchNeedsASession(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	if rec := do(t, h, http.MethodPost, "/api/batches", `{"relation":"none"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: status = %d, want 401", rec.Code)
	}
}

func TestCreateBatchAnswersTheBatch(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	cookie := loginCookie(t, h)

	rec := createBatch(t, h, `{"relation":"parts","name":"Iron Man"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Relation string `json:"relation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Name != "Iron Man" || got.Relation != "parts" {
		t.Errorf("batch = %+v, want the id, name and relation", got)
	}
}

func TestCreateBatchRefusesWhatRF03DoesNotAllow(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	cookie := loginCookie(t, h)

	tests := []struct {
		name string
		body string
	}{
		{name: "no relation", body: `{"name":"x"}`},
		{name: "relation outside RF-03", body: `{"relation":"multipart"}`},
		{name: "name with a separator", body: `{"relation":"none","name":"a/b"}`},
		{name: "not JSON", body: `relation=none`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if rec := createBatch(t, h, tt.body, cookie); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUploadOfABatchIsTrackedAsItsJob(t *testing.T) {
	t.Parallel()

	ingest, conn := newTestIngestion(t)
	log := slog.New(slog.DiscardHandler)
	uploads := newUploads(testConfigFor(t), log, ingest.Service)
	// As in main: tusd blocks the request that completes an upload until its event is taken.
	go uploads.adapter.Run(t.Context())
	h := newTestRoutesWithUploads(t, false, imagecache.New(t.TempDir(), nil), ingest.Handler, uploads.handler, nil)
	cookie := loginCookie(t, h)

	batch := createBatch(t, h, `{"relation":"none"}`, cookie)
	if batch.Code != http.StatusCreated {
		t.Fatalf("create batch = %d, want 201", batch.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(batch.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	res := tusCreateWith(t, h, "filename "+b64("Iron Man (2008).mkv")+",batchId "+b64(created.ID), cookie)
	if res.Code != http.StatusCreated {
		t.Fatalf("create upload = %d, want 201; body %s", res.Code, res.Body.String())
	}
	location := res.Header().Get("Location")
	if rec := tusWrite(t, h, location, "0123456789", cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("write = %d, want 204; body %s", rec.Code, rec.Body.String())
	}

	// The job has the upload's id and sits in the batch, uploaded (RF-03, RF-10a).
	jobID := domain.JobID(location[strings.LastIndex(location, "/")+1:])
	job, err := sqlite.NewJobRepository(conn).Get(t.Context(), jobID)
	if err != nil {
		t.Fatalf("job of the upload: %v", err)
	}
	if job.BatchID != domain.BatchID(created.ID) || job.Status != domain.StatusUploaded || job.FileName != "Iron Man (2008).mkv" {
		t.Errorf("job = %+v, want uploaded in the batch", *job)
	}
}

func TestUploadWithAnUnknownBatchIsRefused(t *testing.T) {
	t.Parallel()

	ingest, _ := newTestIngestion(t)
	uploads := newUploads(testConfigFor(t), slog.New(slog.DiscardHandler), ingest.Service)
	h := newTestRoutesWithUploads(t, false, imagecache.New(t.TempDir(), nil), ingest.Handler, uploads.handler, nil)
	cookie := loginCookie(t, h)

	res := tusCreateWith(t, h, "filename "+b64("a.mkv")+",batchId "+b64("does-not-exist"), cookie)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
}

// b64 encodes a tus metadata value.
func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// testConfigFor is the configuration of a test whose library is a fresh folder.
func testConfigFor(t *testing.T) config.Config {
	t.Helper()
	return config.Config{LibraryPath: t.TempDir()}
}

// newBatchID creates a batch for the session and returns its id.
func newBatchID(t *testing.T, h http.Handler, cookie *http.Cookie) string {
	t.Helper()

	rec := createBatch(t, h, `{"relation":"none"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create batch = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got.ID
}

func TestUploadWithoutABatchIsRefused(t *testing.T) {
	t.Parallel()

	// Every upload belongs to a batch (RF-09a): a file without batchId is refused.
	h := newTestRoutes(t, false)
	cookie := loginCookie(t, h)

	if rec := tusCreateWith(t, h, "filename "+b64("a.mkv"), cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
}
