package tmdb_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/tmdb"
)

func TestImageSourceRequestsTheSizeOfEachKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind domain.ImageKind
		want string // path the CDN must receive
	}{
		{domain.ImagePoster, "/t/p/w500/p.jpg"},
		{domain.ImageBackdrop, "/t/p/w1280/p.jpg"},
		{domain.ImageLogo, "/t/p/w500/p.jpg"},
		{domain.ImageStill, "/t/p/w300/p.jpg"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			var gotPath, gotQuery string
			cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				_, _ = w.Write([]byte("image"))
			}))
			t.Cleanup(cdn.Close)

			src := tmdb.NewImageSource(cdn.Client(), cdn.URL+"/t/p/")
			ref, err := domain.NewImageRef(string(tt.kind), "p.jpg")
			if err != nil {
				t.Fatal(err)
			}
			body, err := src.Fetch(t.Context(), ref)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			defer body.Close()
			if got, _ := io.ReadAll(body); string(got) != "image" {
				t.Errorf("body = %q, want the CDN answer", got)
			}
			if gotPath != tt.want {
				t.Errorf("CDN path = %q, want %q", gotPath, tt.want)
			}
			if gotQuery != "" {
				t.Errorf("CDN query = %q, want none (no credential is sent to the CDN)", gotQuery)
			}
		})
	}
}

func TestImageSourceFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"missing image", http.StatusNotFound, domain.ErrNotFound},
		{"CDN failing", http.StatusInternalServerError, domain.ErrUpstream},
		{"forbidden", http.StatusForbidden, domain.ErrUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(cdn.Close)

			ref, err := domain.NewImageRef("poster", "p.jpg")
			if err != nil {
				t.Fatal(err)
			}
			_, err = tmdb.NewImageSource(cdn.Client(), cdn.URL+"/").Fetch(t.Context(), ref)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestImageSourceUnreachableIsUpstream(t *testing.T) {
	t.Parallel()

	ref, err := domain.NewImageRef("poster", "p.jpg")
	if err != nil {
		t.Fatal(err)
	}
	hc := &http.Client{Transport: unreachableTransport{}}
	_, err = tmdb.NewImageSource(hc, "https://cdn.invalid/").Fetch(t.Context(), ref)
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}
