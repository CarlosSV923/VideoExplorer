package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/broker"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/tus"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// guard bounds the waits of these tests; a healthy run never gets near it.
const guard = 10 * time.Second

// jobJSON is the part of an UploadJob the tests read.
type jobJSON struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	Received int64  `json:"received"`
	Status   string `json:"status"`
	Error    string `json:"error"`
	Batch    struct {
		ID       string `json:"id"`
		Relation string `json:"relation"`
	} `json:"batch"`
}

func TestJobsNeedASession(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	for _, path := range []string{"/api/jobs", "/api/jobs/events"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rec := do(t, h, http.MethodGet, path, "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want the RFC 9457 problem", ct)
			}
		})
	}
}

func TestJobsListTheUploadsWithTheirBatch(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	cookie := loginCookie(t, h)
	batchID := newBatchID(t, h, cookie)
	if rec := tusCreateWith(t, h, "filename "+b64("Iron Man (2008).mkv")+",batchId "+b64(batchID), cookie); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201; body %s", rec.Code, rec.Body.String())
	}

	rec := do(t, h, http.MethodGet, "/api/jobs", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/jobs = %d, want 200", rec.Code)
	}
	var jobs []jobJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("body is not a job list: %v; %s", err, rec.Body.String())
	}
	if len(jobs) != 1 {
		t.Fatalf("%d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.FileName != "Iron Man (2008).mkv" || j.Status != "uploading" || j.Size != 10 || j.Received != 0 {
		t.Errorf("job = %+v, want the uploading file", j)
	}
	if j.Batch.ID != batchID || j.Batch.Relation != "none" {
		t.Errorf("batch = %+v, want %s with relation none", j.Batch, batchID)
	}
}

func TestJobEventsSendTheSnapshotToASession(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	cookie := loginCookie(t, h)
	batchID := newBatchID(t, h, cookie)
	if rec := tusCreateWith(t, h, "filename "+b64("Limbo.mkv")+",batchId "+b64(batchID), cookie); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/jobs/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %d %q, want 200 text/event-stream", res.StatusCode, res.Header.Get("Content-Type"))
	}

	data := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				data <- d
				return
			}
		}
	}()
	select {
	case d := <-data:
		if !strings.Contains(d, `"fileName":"Limbo.mkv"`) {
			t.Errorf("first event = %s, want the job of the upload", d)
		}
	case <-time.After(guard):
		t.Fatal("no event for the job that was already uploading")
	}
}

// tusCreateSized starts an upload of size bytes without a session: the purge
// test mounts the upload endpoint on its own.
func tusCreateSized(t *testing.T, h http.Handler, size int64, uploadMetadata string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, uploadsPath, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
	req.Header.Set("Upload-Metadata", uploadMetadata)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// purgeClock is the clock of the purge test, moved forward by hand.
type purgeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *purgeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *purgeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestPurgeReleasesTheSpaceOfAnAbandonedUpload(t *testing.T) {
	t.Parallel()

	// Real database, staging and purge; only the clock and the free space are set by the test.
	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	log := slog.New(slog.DiscardHandler)
	clk := &purgeClock{now: time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)}
	svc := application.NewService(sqlite.NewBatchRepository(conn), sqlite.NewJobRepository(conn), broker.New(clk.Now), log, clk.Now)
	const size = 1000
	free := domain.FreeSpaceMargin + 1500
	adapter, err := tus.New(stagingDir(t.TempDir()), uploadsPath, log, func(string) (uint64, error) { return free, nil }, svc)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithStaging(adapter)
	uploads := http.StripPrefix(uploadsPath, adapter.Handler())
	batch, err := svc.CreateBatch(t.Context(), "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	meta := "filename " + b64("first.mkv") + ",batchId " + b64(string(batch.ID))

	if rec := tusCreateSized(t, uploads, size, meta); rec.Code != http.StatusCreated {
		t.Fatalf("first create = %d, want 201", rec.Code)
	}
	if rec := tusCreateSized(t, uploads, size, meta); rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("second create while the first reserves its bytes = %d, want 507", rec.Code)
	}

	clk.Advance(application.AbandonedAfter + time.Hour)
	res, err := svc.Purge(t.Context())
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if res.Abandoned != 1 {
		t.Fatalf("purge result %+v, want one abandoned upload", res)
	}
	jobs, err := svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Job.Status != domain.StatusFailed || jobs[0].Job.Error != domain.FailureUploadAbandoned {
		t.Fatalf("job = %+v, want failed upload_abandoned", jobs)
	}

	if rec := tusCreateSized(t, uploads, size, meta); rec.Code != http.StatusCreated {
		t.Fatalf("create after the purge = %d, want 201: the space was not released", rec.Code)
	}
}
