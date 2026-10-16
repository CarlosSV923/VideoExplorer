package http_test

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
)

type downloadHTTPRepo struct {
	titles []domain.LibraryTitle
	files  map[int64][]domain.LibraryFile
}

func (r downloadHTTPRepo) TitleByFolder(context.Context, string) (domain.LibraryTitle, bool, error) {
	return domain.LibraryTitle{}, false, nil
}

func (r downloadHTTPRepo) Titles(context.Context) ([]domain.LibraryTitle, error) {
	return r.titles, nil
}

func (r downloadHTTPRepo) Files(_ context.Context, id int64) ([]domain.LibraryFile, error) {
	return r.files[id], nil
}

func serveLibraryFile(t *testing.T, req *http.Request, h *catalogshttp.LibraryHandler, object httpapi.DownloadLibraryFileRequestObject) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	httpx.WithRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, err := h.DownloadLibraryFile(r.Context(), object)
		if err != nil {
			t.Errorf("DownloadLibraryFile: %v", err)
			return
		}
		if err := response.VisitDownloadLibraryFileResponse(w); err != nil {
			t.Errorf("visit response: %v", err)
		}
	})).ServeHTTP(rr, req)
	return rr
}

func TestLibraryDownloadServesRangeAndRFC6266Name(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := libraryfs.New(root)
	rel := "Movies/Película (2020)/Película.mkv"
	p := files.LibraryPath(filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := downloadHTTPRepo{
		titles: []domain.LibraryTitle{{ID: 1, Kind: domain.KindMovie, Title: "Película", DiscTitle: "Película", Folder: "Movies/Película (2020)"}},
		files:  map[int64][]domain.LibraryFile{1: {{ID: 7, TitleID: 1, Kind: domain.FileVideo, Rel: rel}}},
	}
	h := catalogshttp.NewLibraryHandler(application.NewLibraryViews(repo, nil, nil), nil, nil, nil, application.NewLibraryDownloads(repo, files))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/library/titles/1/files/7/download", nil)
	req.Header.Set("Range", "bytes=2-5")
	rr := serveLibraryFile(t, req, h, httpapi.DownloadLibraryFileRequestObject{Id: 1, FileId: 7})
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "2345" {
		t.Fatalf("status/body = %d/%q, want 206/2345", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Errorf("Content-Range = %q", got)
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, "filename=Pelicula.mkv") || !strings.Contains(got, "filename*=UTF-8''Pel%C3%ADcula.mkv") {
		t.Errorf("Content-Disposition = %q", got)
	}
}

func TestLibraryDownloadTitleStreamsZipWithExactLength(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := libraryfs.New(root)
	rel := "Movies/Godzilla (2014)/Godzilla.mkv"
	p := files.LibraryPath(filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := downloadHTTPRepo{
		titles: []domain.LibraryTitle{{ID: 1, Kind: domain.KindMovie, Title: "Godzilla", DiscTitle: "Godzilla", Folder: "Movies/Godzilla (2014)"}},
		files:  map[int64][]domain.LibraryFile{1: {{ID: 7, TitleID: 1, Kind: domain.FileVideo, Rel: rel}}},
	}
	h := catalogshttp.NewLibraryHandler(application.NewLibraryViews(repo, nil, nil), nil, nil, nil, application.NewLibraryDownloads(repo, files))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/library/titles/1/download", nil)
	rr := httptest.NewRecorder()
	httpx.WithRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, err := h.DownloadLibraryTitle(r.Context(), httpapi.DownloadLibraryTitleRequestObject{Id: 1})
		if err != nil {
			t.Errorf("DownloadLibraryTitle: %v", err)
			return
		}
		if err := response.VisitDownloadLibraryTitleResponse(w); err != nil {
			t.Errorf("visit response: %v", err)
		}
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("status/content type = %d/%q", rr.Code, rr.Header().Get("Content-Type"))
	}
	if got := rr.Header().Get("Content-Length"); got != strconv.Itoa(rr.Body.Len()) {
		t.Fatalf("Content-Length = %q, body length = %d", got, rr.Body.Len())
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, "Godzilla (2014).zip") {
		t.Errorf("Content-Disposition = %q", got)
	}
	zr, err := zip.NewReader(strings.NewReader(rr.Body.String()), int64(rr.Body.Len()))
	if err != nil || len(zr.File) != 1 || zr.File[0].Name != "Godzilla (2014)/Godzilla.mkv" {
		t.Fatalf("zip = %v, files=%v", err, zr.File)
	}
}

func TestLibraryDownloadUnknownFileReturnsNotFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := libraryfs.New(root)
	repo := downloadHTTPRepo{
		titles: []domain.LibraryTitle{{ID: 1, Kind: domain.KindMovie, Title: "Gone", Folder: "Movies/Gone"}},
		files:  map[int64][]domain.LibraryFile{1: {{ID: 7, TitleID: 1, Kind: domain.FileVideo, Rel: "Movies/Gone/Gone.mkv", TrashEntryID: 42}}},
	}
	h := catalogshttp.NewLibraryHandler(application.NewLibraryViews(repo, nil, nil), nil, nil, nil, application.NewLibraryDownloads(repo, files))
	response, err := h.DownloadLibraryFile(t.Context(), httpapi.DownloadLibraryFileRequestObject{Id: 1, FileId: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(httpapi.DownloadLibraryFile404ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %#v, want 404 not_found", response)
	}
}

func TestUnassignedDownloadEntryFileAndCopyingConflict(t *testing.T) {
	t.Parallel()
	env := setupSection(t)
	env.put(t, "Ready/movie.mkv", env.now.Add(-2*time.Minute))
	env.put(t, "Ready/movie.es.srt", env.now.Add(-2*time.Minute))
	id := env.idOf(t, "Ready")

	fileRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/unassigned/"+strconv.FormatInt(id, 10)+"/files/download?path=movie.mkv", nil)
	fileRequest.Header.Set("Range", "bytes=1-3")
	fileRR := httptest.NewRecorder()
	httpx.WithRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileResponse, err := env.section.DownloadUnassignedFile(r.Context(), httpapi.DownloadUnassignedFileRequestObject{Id: id, Params: httpapi.DownloadUnassignedFileParams{Path: "movie.mkv"}})
		if err != nil {
			t.Errorf("DownloadUnassignedFile: %v", err)
			return
		}
		if err := fileResponse.VisitDownloadUnassignedFileResponse(w); err != nil {
			t.Errorf("visit file response: %v", err)
		}
	})).ServeHTTP(fileRR, fileRequest)
	if fileRR.Code != http.StatusPartialContent || fileRR.Body.String() != "ont" {
		t.Fatalf("file response = %d/%q, want 206/ont", fileRR.Code, fileRR.Body.String())
	}

	entryResponse, err := env.section.DownloadUnassignedEntry(t.Context(), httpapi.DownloadUnassignedEntryRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	if err := entryResponse.VisitDownloadUnassignedEntryResponse(rr); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/zip" || rr.Body.Len() == 0 {
		t.Fatalf("entry response = %d/%q/%d", rr.Code, rr.Header().Get("Content-Type"), rr.Body.Len())
	}

	env.put(t, "Copy/movie.mkv", env.now.Add(-5*time.Second))
	copyID := env.idOf(t, "Copy")
	conflict, err := env.section.DownloadUnassignedEntry(t.Context(), httpapi.DownloadUnassignedEntryRequestObject{Id: copyID})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := conflict.(httpapi.DownloadUnassignedEntry409ApplicationProblemPlusJSONResponse); !ok || got.Code != "copying" {
		t.Fatalf("copying response = %#v, want 409 copying", conflict)
	}
}
