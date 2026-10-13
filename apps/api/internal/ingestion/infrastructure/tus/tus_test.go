package tus_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/tus"
)

const (
	uploadsPath = "/api/uploads/"
	// testBatch is a batch id the tests upload into (RF-09a: every upload names one).
	testBatch = "lote-1"
	// guard bounds every wait on a background event. It only matters when
	// something is broken; a healthy run never gets near it.
	guard = 10 * time.Second
)

// fakeUploads records what the adapter asks of the ingestion context and hands
// out job ids. err, when set, is what Admit answers.
type fakeUploads struct {
	mu         sync.Mutex
	admissions []application.Admission
	progress   []int64
	err        error
	// refuse is what Terminating answers.
	refuse     error
	completed  chan domain.JobID
	terminated chan domain.JobID
}

func (f *fakeUploads) Admit(_ context.Context, a application.Admission) (domain.JobID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.admissions = append(f.admissions, a)
	return domain.JobID(fmt.Sprintf("job-%d", len(f.admissions))), nil
}

func (f *fakeUploads) admitted() []application.Admission {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]application.Admission(nil), f.admissions...)
}

func (f *fakeUploads) Completed(_ context.Context, id domain.JobID) error {
	f.completed <- id
	return nil
}

func (f *fakeUploads) UploadProgress(_ context.Context, _ domain.JobID, received int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress = append(f.progress, received)
	return nil
}

func (f *fakeUploads) received() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.progress...)
}

func (f *fakeUploads) Terminated(_ context.Context, id domain.JobID) error {
	f.terminated <- id
	return nil
}

// Terminating answers refuse, when set, for the deletions it is asked about.
func (f *fakeUploads) Terminating(_ context.Context, _ domain.JobID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refuse
}

// fixture is a tus endpoint mounted as the production routes mount it.
type fixture struct {
	dir     string
	srv     *httptest.Server
	uploads *fakeUploads
	adapter *tus.Adapter
}

// newFixture starts the endpoint with free bytes on its file system.
func newFixture(t *testing.T, free uint64) fixture {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "staging")
	uploads := &fakeUploads{completed: make(chan domain.JobID, 16), terminated: make(chan domain.JobID, 16)}
	a, err := tus.New(dir, uploadsPath, nil, func(string) (uint64, error) { return free, nil }, uploads)
	if err != nil {
		t.Fatal(err)
	}
	go a.Run(t.Context())

	mux := http.NewServeMux()
	mux.Handle(uploadsPath, http.StripPrefix(uploadsPath, a.Handler()))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fixture{dir: dir, srv: srv, uploads: uploads, adapter: a}
}

// do sends a request and closes the body when the test ends.
func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// metadata builds an Upload-Metadata header from key/value pairs.
func metadata(kv ...string) string {
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, kv[i]+" "+base64.StdEncoding.EncodeToString([]byte(kv[i+1])))
	}
	return strings.Join(parts, ",")
}

// request builds a request for a tus URL. The Location header tusd returns is
// absolute, and clients use it as it comes.
func request(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// create starts an upload of size bytes and returns the response.
func (f fixture) create(t *testing.T, size int64, uploadMetadata string) *http.Response {
	t.Helper()

	req := request(t, http.MethodPost, f.srv.URL+uploadsPath, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
	if uploadMetadata != "" {
		req.Header.Set("Upload-Metadata", uploadMetadata)
	}
	return do(t, req)
}

// patch sends one chunk at offset and returns the response.
func (f fixture) patch(t *testing.T, location string, offset int64, chunk string) *http.Response {
	t.Helper()

	req := request(t, http.MethodPatch, location, strings.NewReader(chunk))
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	return do(t, req)
}

// head asks the server how many bytes it has (the resume point).
func (f fixture) head(t *testing.T, location string) *http.Response {
	t.Helper()

	req := request(t, http.MethodHead, location, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	return do(t, req)
}

// remove deletes the upload (tus termination).
func (f fixture) remove(t *testing.T, location string) *http.Response {
	t.Helper()

	req := request(t, http.MethodDelete, location, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	return do(t, req)
}

// awaitCompleted waits for the next finished upload and returns its job id.
func (f fixture) awaitCompleted(t *testing.T) domain.JobID {
	t.Helper()

	select {
	case id := <-f.uploads.completed:
		return id
	case <-time.After(guard):
		t.Fatal("the finished upload was not reported")
		return ""
	}
}

// awaitTerminated waits for the next deleted upload and returns its job id.
func (f fixture) awaitTerminated(t *testing.T) domain.JobID {
	t.Helper()

	select {
	case id := <-f.uploads.terminated:
		return id
	case <-time.After(guard):
		t.Fatal("the deleted upload was not reported")
		return ""
	}
}

// idOf returns the upload id at the end of a Location header.
func idOf(location string) string {
	return location[strings.LastIndex(location, "/")+1:]
}

func TestUploadIsStoredUnderItsJobAndReported(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	res := f.create(t, 10, metadata(
		"filename", "  Iron Man (2008).mkv ",
		"relativePath", "Iron Man/Iron Man (2008).mkv",
		"batchId", "lote-1",
	))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", res.StatusCode)
	}
	location := res.Header.Get("Location")
	if !strings.Contains(location, uploadsPath) {
		t.Fatalf("Location = %q, want a URL under /api/uploads/", location)
	}

	// The upload id is the id of its job: the browser and the server name it alike.
	admitted := f.uploads.admitted()
	if len(admitted) != 1 {
		t.Fatalf("%d admissions, want 1", len(admitted))
	}
	want := application.Admission{
		Meta: domain.UploadMeta{FileName: "Iron Man (2008).mkv", RelativePath: "Iron Man/Iron Man (2008).mkv", BatchID: "lote-1"},
		Size: 10,
	}
	if admitted[0] != want {
		t.Errorf("admission = %+v, want %+v", admitted[0], want)
	}

	res = f.patch(t, location, 0, "0123456789")
	if res.StatusCode != http.StatusNoContent || res.Header.Get("Upload-Offset") != "10" {
		t.Fatalf("patch = %d offset %q, want 204 at 10", res.StatusCode, res.Header.Get("Upload-Offset"))
	}

	if id := f.awaitCompleted(t); string(id) != idOf(location) || idOf(location) != "job-1" {
		t.Errorf("completed job = %q, want the id in Location %q", id, idOf(location))
	}
	data, err := os.ReadFile(filepath.Join(f.dir, idOf(location)))
	if err != nil || string(data) != "0123456789" {
		t.Fatalf("stored bytes = %q, %v", data, err)
	}
}

func TestInterruptedUploadResumesFromItsOffset(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	res := f.create(t, 10, metadata("filename", "a.mkv", "batchId", testBatch))
	location := res.Header.Get("Location")

	// The first request is cut after five bytes: the browser keeps no state.
	if res := f.patch(t, location, 0, "01234"); res.Header.Get("Upload-Offset") != "5" {
		t.Fatalf("first chunk offset = %q, want 5", res.Header.Get("Upload-Offset"))
	}

	// After the cut the browser asks where to resume (HEAD), then sends the rest.
	res = f.head(t, location)
	if res.StatusCode != http.StatusOK || res.Header.Get("Upload-Offset") != "5" || res.Header.Get("Upload-Length") != "10" {
		t.Fatalf("head = %d offset %q length %q, want 200 at 5 of 10",
			res.StatusCode, res.Header.Get("Upload-Offset"), res.Header.Get("Upload-Length"))
	}
	if res := f.patch(t, location, 3, "xxx"); res.StatusCode != http.StatusConflict {
		t.Fatalf("patch at a wrong offset = %d, want 409", res.StatusCode)
	}
	res = f.patch(t, location, 5, "56789")
	if res.StatusCode != http.StatusNoContent || res.Header.Get("Upload-Offset") != "10" {
		t.Fatalf("resumed patch = %d offset %q, want 204 at 10", res.StatusCode, res.Header.Get("Upload-Offset"))
	}

	f.awaitCompleted(t)
	if data, err := os.ReadFile(filepath.Join(f.dir, idOf(location))); err != nil || string(data) != "0123456789" {
		t.Fatalf("stored bytes = %q, %v", data, err)
	}
}

func TestDeletedUploadIsReportedWithItsJob(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	location := f.create(t, 10, metadata("filename", "a.mkv", "batchId", testBatch)).Header.Get("Location")

	if res := f.remove(t, location); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", res.StatusCode)
	}
	if id := f.awaitTerminated(t); string(id) != idOf(location) {
		t.Errorf("terminated job = %q, want %q", id, idOf(location))
	}
}

func TestDeletionOfAnOriginalWaitingForItsTrashIsRefusedAndNothingIsDeleted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		refuse  error
		want    int
		deleted bool
	}{
		{name: "an upload whose original waits for the trash is kept", refuse: application.ErrOriginalPending, want: http.StatusConflict},
		{name: "an upload that may go is deleted", want: http.StatusNoContent, deleted: true},
		{name: "a failed check keeps the upload", refuse: errors.New("database is busy"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, 50<<30)
			f.uploads.mu.Lock()
			f.uploads.refuse = tt.refuse
			f.uploads.mu.Unlock()
			location := f.create(t, 10, metadata("filename", "a.zip", "batchId", testBatch)).Header.Get("Location")
			staged := filepath.Join(f.dir, idOf(location))
			if _, err := os.Stat(staged); err != nil {
				t.Fatalf("the staged file is missing before the deletion: %v", err)
			}

			if res := f.remove(t, location); res.StatusCode != tt.want {
				t.Fatalf("delete = %d, want %d", res.StatusCode, tt.want)
			}
			_, err := os.Stat(staged)
			if tt.deleted != errors.Is(err, fs.ErrNotExist) {
				t.Errorf("staged file deleted = %v, want %v", errors.Is(err, fs.ErrNotExist), tt.deleted)
			}
		})
	}
}

func TestCreateChecksFreeSpaceWithTheMargin(t *testing.T) {
	t.Parallel()

	const size = 1000
	need := uint64(size) + domain.FreeSpaceMargin
	tests := []struct {
		name       string
		free       uint64
		wantStatus int
	}{
		{name: "exactly the size plus the margin", free: need, wantStatus: http.StatusCreated},
		{name: "one byte short", free: need - 1, wantStatus: http.StatusInsufficientStorage},
		{name: "no space at all", free: 0, wantStatus: http.StatusInsufficientStorage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.free)
			res := f.create(t, size, metadata("filename", "a.mkv", "batchId", testBatch))
			if res.StatusCode != tt.wantStatus {
				t.Fatalf("create status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusInsufficientStorage {
				if n := stagedUploads(t, f.dir); n != 0 {
					t.Errorf("%d upload(s) were staged despite the refusal", n)
				}
				if n := len(f.uploads.admitted()); n != 0 {
					t.Errorf("%d job(s) were admitted despite the refusal", n)
				}
			}
		})
	}
}

func TestCreateReservesTheBytesOfUploadsInProgress(t *testing.T) {
	t.Parallel()

	// Room for two uploads of 1000 bytes only when the first one has received
	// half of its bytes: the bytes still to come are not free (RF-09a).
	const size = 1000
	f := newFixture(t, domain.FreeSpaceMargin+1500)

	first := f.create(t, size, metadata("filename", "first.mkv", "batchId", testBatch))
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first create = %d, want 201", first.StatusCode)
	}
	firstLocation := first.Header.Get("Location")

	second := f.create(t, size, metadata("filename", "second.mkv", "batchId", testBatch))
	if second.StatusCode != http.StatusInsufficientStorage {
		t.Fatalf("second create with the first one still empty = %d, want 507", second.StatusCode)
	}
	if n := stagedUploads(t, f.dir); n != 1 {
		t.Errorf("%d staged upload(s) after the refusal, want 1", n)
	}

	if res := f.patch(t, firstLocation, 0, strings.Repeat("x", size/2)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("half of the first upload = %d, want 204", res.StatusCode)
	}
	second = f.create(t, size, metadata("filename", "second.mkv", "batchId", testBatch))
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("second create after half of the first arrived = %d, want 201", second.StatusCode)
	}
	if n := len(f.uploads.admitted()); n != 2 {
		t.Errorf("%d jobs admitted, want 2", n)
	}
}

func TestCreateRefusesAnEmptyFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	if res := f.create(t, 0, metadata("filename", "a.mkv", "batchId", testBatch)); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("0-byte create status = %d, want 400", res.StatusCode)
	}
	if n := stagedUploads(t, f.dir); n != 0 {
		t.Errorf("%d upload(s) were staged for an empty file", n)
	}
	if n := len(f.uploads.admitted()); n != 0 {
		t.Errorf("%d job(s) were admitted for an empty file", n)
	}
}

func TestCreateRefusesABatchThatDoesNotExist(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	f.uploads.err = domain.ErrBatchNotFound
	res := f.create(t, 10, metadata("filename", "a.mkv", "batchId", "nope"))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown batch status = %d, want 400", res.StatusCode)
	}
	if n := stagedUploads(t, f.dir); n != 0 {
		t.Errorf("%d upload(s) were staged for an unknown batch", n)
	}
}

func TestCreateRefusesInvalidMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata string
	}{
		{name: "no filename", metadata: metadata("batchId", "b1")},
		{name: "no batch id", metadata: metadata("filename", "a.mkv")},
		{name: "empty batch id", metadata: metadata("filename", "a.mkv", "batchId", "")},
		{name: "parent folder as name", metadata: metadata("filename", "..", "batchId", testBatch)},
		{name: "path in the file name", metadata: metadata("filename", "../etc/passwd", "batchId", testBatch)},
		{name: "absolute relative path", metadata: metadata("filename", "a.mkv", "relativePath", "/etc/a.mkv", "batchId", testBatch)},
		{name: "relative path with parent", metadata: metadata("filename", "a.mkv", "relativePath", "x/../a.mkv", "batchId", testBatch)},
		{name: "relative path not ending in the file name", metadata: metadata("filename", "a.mkv", "relativePath", "x/b.mkv", "batchId", testBatch)},
		{name: "batch id with a path", metadata: metadata("filename", "a.mkv", "batchId", "../b")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, 50<<30)
			if res := f.create(t, 10, tt.metadata); res.StatusCode != http.StatusBadRequest {
				t.Fatalf("create status = %d, want 400", res.StatusCode)
			}
			if n := stagedUploads(t, f.dir); n != 0 {
				t.Errorf("%d upload(s) were staged despite the refusal", n)
			}
			if n := len(f.uploads.admitted()); n != 0 {
				t.Errorf("%d job(s) were admitted despite the refusal", n)
			}
		})
	}
}

func TestCreateRequiresTheSizeUpFront(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	req := request(t, http.MethodPost, f.srv.URL+uploadsPath, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Defer-Length", "1")
	req.Header.Set("Upload-Metadata", metadata("filename", "a.mkv", "batchId", testBatch))
	if res := do(t, req); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("deferred length status = %d, want 400", res.StatusCode)
	}
}

// stagedUploads counts the upload records in the staging folder.
func stagedUploads(t *testing.T, dir string) int {
	t.Helper()

	infos, err := filepath.Glob(filepath.Join(dir, "*.info"))
	if err != nil {
		t.Fatal(err)
	}
	return len(infos)
}
