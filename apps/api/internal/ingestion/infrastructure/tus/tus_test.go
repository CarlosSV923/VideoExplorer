package tus_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/tus"
)

const (
	uploadsPath = "/api/uploads/"
	// guard bounds every wait on a background event. It only matters when
	// something is broken; a healthy run never gets near it.
	guard = 10 * time.Second
)

// fixture is a tus endpoint mounted as the production routes mount it.
type fixture struct {
	dir  string
	srv  *httptest.Server
	done chan application.CompletedUpload
}

// completedRecorder records every finished upload.
type completedRecorder struct {
	done chan application.CompletedUpload
}

func (r completedRecorder) HandleUploadCompleted(_ context.Context, u application.CompletedUpload) error {
	r.done <- u
	return nil
}

// newFixture starts the endpoint with free bytes on its file system. next may
// be nil; then the recorder is not connected.
func newFixture(t *testing.T, free uint64, connect bool) fixture {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "staging")
	done := make(chan application.CompletedUpload, 16)
	var next application.UploadCompletedHandler
	if connect {
		next = completedRecorder{done: done}
	}
	a, err := tus.New(dir, uploadsPath, nil, func(string) (uint64, error) { return free, nil }, next)
	if err != nil {
		t.Fatal(err)
	}
	go a.Run(t.Context())

	mux := http.NewServeMux()
	mux.Handle(uploadsPath, http.StripPrefix(uploadsPath, a.Handler()))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fixture{dir: dir, srv: srv, done: done}
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

// awaitCompleted waits for the next finished upload.
func (f fixture) awaitCompleted(t *testing.T) application.CompletedUpload {
	t.Helper()

	select {
	case u := <-f.done:
		return u
	case <-time.After(guard):
		t.Fatal("the finished upload was not handed over")
		return application.CompletedUpload{}
	}
}

// idOf returns the upload id at the end of a Location header.
func idOf(location string) string {
	return location[strings.LastIndex(location, "/")+1:]
}

func TestUploadIsStoredAndHandedOver(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30, true)
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

	res = f.patch(t, location, 0, "0123456789")
	if res.StatusCode != http.StatusNoContent || res.Header.Get("Upload-Offset") != "10" {
		t.Fatalf("patch = %d offset %q, want 204 at 10", res.StatusCode, res.Header.Get("Upload-Offset"))
	}

	u := f.awaitCompleted(t)
	id := idOf(location)
	if u.ID != id || u.Size != 10 {
		t.Errorf("completed id/size = %q/%d, want %q/10", u.ID, u.Size, id)
	}
	want := domain.UploadMeta{FileName: "Iron Man (2008).mkv", RelativePath: "Iron Man/Iron Man (2008).mkv", BatchID: "lote-1"}
	if u.Meta != want {
		t.Errorf("meta = %+v, want %+v", u.Meta, want)
	}
	if u.StoredAt != filepath.Join(f.dir, id) {
		t.Errorf("stored at %q, want the staging folder", u.StoredAt)
	}
	data, err := readFile(u.StoredAt)
	if err != nil || data != "0123456789" {
		t.Fatalf("stored bytes = %q, %v", data, err)
	}
}

func TestInterruptedUploadResumesFromItsOffset(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30, true)
	res := f.create(t, 10, metadata("filename", "a.mkv"))
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

	u := f.awaitCompleted(t)
	if u.Size != 10 {
		t.Errorf("completed size = %d, want 10", u.Size)
	}
	if data, err := readFile(u.StoredAt); err != nil || data != "0123456789" {
		t.Fatalf("stored bytes = %q, %v", data, err)
	}
}

func TestFinishedUploadWithoutHandlerDoesNotBlock(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30, false)
	location := f.create(t, 4, metadata("filename", "a.mkv")).Header.Get("Location")

	if res := f.patch(t, location, 0, "abcd"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("patch without a handler = %d, want 204", res.StatusCode)
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

			f := newFixture(t, tt.free, false)
			res := f.create(t, size, metadata("filename", "a.mkv"))
			if res.StatusCode != tt.wantStatus {
				t.Fatalf("create status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusInsufficientStorage {
				if n := stagedUploads(t, f.dir); n != 0 {
					t.Errorf("%d upload(s) were staged despite the refusal", n)
				}
			}
		})
	}
}

func TestCreateRefusesInvalidMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata string
	}{
		{name: "no filename", metadata: metadata("batchId", "b1")},
		{name: "parent folder as name", metadata: metadata("filename", "..")},
		{name: "path in the file name", metadata: metadata("filename", "../etc/passwd")},
		{name: "absolute relative path", metadata: metadata("filename", "a.mkv", "relativePath", "/etc/a.mkv")},
		{name: "relative path with parent", metadata: metadata("filename", "a.mkv", "relativePath", "x/../a.mkv")},
		{name: "relative path not ending in the file name", metadata: metadata("filename", "a.mkv", "relativePath", "x/b.mkv")},
		{name: "batch id with a path", metadata: metadata("filename", "a.mkv", "batchId", "../b")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, 50<<30, false)
			if res := f.create(t, 10, tt.metadata); res.StatusCode != http.StatusBadRequest {
				t.Fatalf("create status = %d, want 400", res.StatusCode)
			}
			if n := stagedUploads(t, f.dir); n != 0 {
				t.Errorf("%d upload(s) were staged despite the refusal", n)
			}
		})
	}
}

func TestCreateRequiresTheSizeUpFront(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30, false)
	req := request(t, http.MethodPost, f.srv.URL+uploadsPath, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Defer-Length", "1")
	req.Header.Set("Upload-Metadata", metadata("filename", "a.mkv"))
	if res := do(t, req); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("deferred length status = %d, want 400", res.StatusCode)
	}
}

// readFile returns the content of a file as a string.
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
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
