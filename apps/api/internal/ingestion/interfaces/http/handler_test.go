package http

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/broker"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// guard bounds every wait of the tests; a healthy run never gets near it.
const guard = 10 * time.Second

var start = time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

// fixture is the ingestion handler over a real database, served by a test server
// that streams its events the way the generated router does.
type fixture struct {
	svc      *application.Service
	handler  *Handler
	shutdown chan struct{}
	server   *httptest.Server
	batch    domain.BatchID
	clock    *testClock
	// ended receives once the stream handler has returned.
	ended chan struct{}
}

// testClock is a time the test moves forward by hand.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newFixture starts the server with a heartbeat of heartbeatEvery.
func newFixture(t *testing.T, heartbeatEvery time.Duration) fixture {
	t.Helper()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	clk := &testClock{now: start}
	events := broker.New(clk.Now)
	shutdown := make(chan struct{})
	svc := application.NewService(sqlite.NewBatchRepository(conn), sqlite.NewJobRepository(conn), events, slog.New(slog.DiscardHandler), clk.Now)
	h := NewHandler(svc, events, shutdown, slog.New(slog.DiscardHandler))
	h.heartbeat = heartbeatEvery

	f := fixture{svc: svc, handler: h, shutdown: shutdown, ended: make(chan struct{}), clock: clk}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obj, err := h.StreamJobEvents(r.Context(), httpapi.StreamJobEventsRequestObject{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = obj.VisitStreamJobEventsResponse(w)
		close(f.ended)
	}))
	t.Cleanup(f.server.Close)

	batch, err := svc.CreateBatch(t.Context(), "Show", domain.RelationEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	f.batch = batch.ID
	return f
}

// admit starts an upload of 10 bytes named name, in the test's batch.
func (f fixture) admit(t *testing.T, name string) domain.JobID {
	t.Helper()

	id, err := f.svc.Admit(t.Context(), application.Admission{
		Meta: domain.UploadMeta{FileName: name, BatchID: string(f.batch)},
		Size: 10,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	return id
}

// stream is a connected event stream: its events arrive on the channel.
type stream struct {
	events chan httpapi.UploadJob
	lines  chan string
	body   io.ReadCloser
}

// connect opens the event stream and reads what it sends.
func (f fixture) connect(t *testing.T) stream {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	s := stream{events: make(chan httpapi.UploadJob, 256), lines: make(chan string, 256), body: res.Body}
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			line := sc.Text()
			s.lines <- line
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				var j httpapi.UploadJob
				if err := json.Unmarshal([]byte(data), &j); err == nil {
					s.events <- j
				}
			}
		}
		close(s.events)
		close(s.lines)
	}()
	return s
}

// next returns the next job event of the stream.
func (s stream) next(t *testing.T) httpapi.UploadJob {
	t.Helper()

	select {
	case j, ok := <-s.events:
		if !ok {
			t.Fatal("the stream ended before the next event")
		}
		return j
	case <-time.After(guard):
		t.Fatal("no event within the guard time")
		return httpapi.UploadJob{}
	}
}

func TestStreamReplaysTheListThenSendsEachChange(t *testing.T) {
	t.Parallel()

	f := newFixture(t, time.Hour)
	older := f.admit(t, "E01.mkv")
	newer := f.admit(t, "E02.mkv")

	s := f.connect(t)
	if got := s.next(t); got.Id != string(newer) || got.Status != httpapi.Uploading || got.Batch.Name != "Show" {
		t.Errorf("first snapshot event = %+v, want the newest job with its batch", got)
	}
	if got := s.next(t); got.Id != string(older) {
		t.Errorf("second snapshot event = %s, want the older job", got.Id)
	}

	// Progress is limited to about two events a second per job: wait out the interval.
	f.clock.Advance(time.Second)
	if err := f.svc.UploadProgress(t.Context(), older, 6); err != nil {
		t.Fatal(err)
	}
	if got := s.next(t); got.Id != string(older) || got.Received != 6 || got.Size != 10 {
		t.Errorf("progress event = %+v, want 6 of 10 bytes", got)
	}
	if err := f.svc.Completed(t.Context(), older); err != nil {
		t.Fatal(err)
	}
	if got := s.next(t); got.Id != string(older) || got.Status != httpapi.Uploaded || got.Received != 10 {
		t.Errorf("status event = %+v, want uploaded with all bytes", got)
	}
}

func TestStreamSendsEventsNamedJobWithTheWholeJob(t *testing.T) {
	t.Parallel()

	f := newFixture(t, time.Hour)
	f.admit(t, "a.mkv")
	s := f.connect(t)
	s.next(t)

	var sawEvent, sawData bool
	deadline := time.After(guard)
	for !sawEvent || !sawData {
		select {
		case line := <-s.lines:
			switch {
			case line == "event: job":
				sawEvent = true
			case strings.HasPrefix(line, "data: {") && strings.Contains(line, `"fileName":"a.mkv"`):
				sawData = true
			}
		case <-deadline:
			t.Fatalf("event: job = %v, data = %v: the frame is not in the SSE format", sawEvent, sawData)
		}
	}
}

func TestStreamSendsAKeepAliveWhileIdle(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 10*time.Millisecond)
	s := f.connect(t)

	deadline := time.After(guard)
	for {
		select {
		case line := <-s.lines:
			if line == ": ping" {
				return
			}
		case <-deadline:
			t.Fatal("no keep-alive comment while the stream was idle")
		}
	}
}

func TestStreamEndsWhenTheServerShutsDown(t *testing.T) {
	t.Parallel()

	f := newFixture(t, time.Hour)
	s := f.connect(t)
	close(f.shutdown)

	// The body ends, so the server is not kept waiting for the stream at shutdown.
	// The lines channel closes when the reader of the body ends.
	done := make(chan struct{})
	go func() {
		for range s.lines {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(guard):
		t.Fatal("the event stream was still open after the server started shutting down")
	}
	select {
	case <-f.ended:
	case <-time.After(guard):
		t.Fatal("the stream handler did not return after the shutdown")
	}
}

func TestStreamEndsWhenTheClientGoesAway(t *testing.T) {
	t.Parallel()

	f := newFixture(t, time.Hour)
	s := f.connect(t)
	if err := s.body.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-f.ended:
	case <-time.After(guard):
		t.Fatal("the stream handler kept running after the client went away")
	}
}

func TestListJobsAnswersWithTheBatchOfEachJob(t *testing.T) {
	t.Parallel()

	f := newFixture(t, time.Hour)
	id := f.admit(t, "E01.mkv")

	views, err := f.svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("List returned %d jobs, want 1", len(views))
	}
	got := JobToAPI(views[0])
	if got.Id != string(id) || got.Batch.Id != string(f.batch) || got.Batch.Relation != httpapi.BatchRelation(domain.RelationEpisodes) {
		t.Errorf("job = %+v, want the job with its batch", got)
	}
	if got.Error != nil {
		t.Errorf("error = %q for a job that did not fail, want none", *got.Error)
	}
}
