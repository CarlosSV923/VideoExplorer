package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

func TestParseImageKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    domain.ImageKind
		wantErr bool
	}{
		{in: "poster", want: domain.ImagePoster},
		{in: "backdrop", want: domain.ImageBackdrop},
		{in: "logo", want: domain.ImageLogo},
		{in: "still", want: domain.ImageStill},
		{in: "", wantErr: true},
		{in: "Poster", wantErr: true},
		{in: "avatar", wantErr: true},
		{in: "poster/../logo", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseImageKind(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrInvalidImage) {
				t.Errorf("err = %v, want ErrInvalidImage", err)
			}
			if got != tt.want {
				t.Errorf("kind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewImageRefValidatesFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{name: "tmdb jpg", file: "9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg"},
		{name: "jpeg", file: "a_b-c.jpeg"},
		{name: "png", file: "logo.png"},
		{name: "upper case extension", file: "a.JPG", wantErr: true},
		{name: "svg not served yet", file: "logo.svg", wantErr: true},
		{name: "other extension", file: "a.gif", wantErr: true},
		{name: "no extension", file: "a", wantErr: true},
		{name: "dot dot", file: "..", wantErr: true},
		{name: "traversal", file: "../secret.jpg", wantErr: true},
		{name: "separator", file: "a/b.jpg", wantErr: true},
		{name: "backslash", file: `a\b.jpg`, wantErr: true},
		{name: "leading slash", file: "/a.jpg", wantErr: true},
		{name: "query", file: "a.jpg?x=1", wantErr: true},
		{name: "dot file", file: ".jpg", wantErr: true},
		{name: "empty", file: "", wantErr: true},
		{name: "too long", file: strings.Repeat("a", 129) + ".jpg", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ref, err := domain.NewImageRef("poster", tt.file)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !errors.Is(err, domain.ErrInvalidImage) {
					t.Errorf("err = %v, want ErrInvalidImage", err)
				}
				return
			}
			if ref.File != tt.file || ref.Kind != domain.ImagePoster {
				t.Errorf("ref = %+v, want poster %q", ref, tt.file)
			}
		})
	}
}

func TestNewImageRefRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	if _, err := domain.NewImageRef("avatar", "a.jpg"); !errors.Is(err, domain.ErrInvalidImage) {
		t.Fatalf("err = %v, want ErrInvalidImage", err)
	}
}

func TestImageRefContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file string
		want string
	}{
		{file: "a.jpg", want: "image/jpeg"},
		{file: "a.jpeg", want: "image/jpeg"},
		{file: "a.png", want: "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			ref, err := domain.NewImageRef("logo", tt.file)
			if err != nil {
				t.Fatal(err)
			}
			if got := ref.ContentType(); got != tt.want {
				t.Errorf("ContentType = %q, want %q", got, tt.want)
			}
		})
	}
}
