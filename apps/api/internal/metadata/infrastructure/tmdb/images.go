package tmdb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// imageBase is TMDB's image CDN. The app never lets the browser reach it:
// the URLs are only for the local image cache (spec §6, RNF-10).
const imageBase = "https://image.tmdb.org/t/p/"

// imageSizes are the sizes the app downloads (spec §6): poster and logo w500,
// backdrop w1280, episode still w300.
var imageSizes = map[domain.ImageKind]string{
	domain.ImagePoster:   "w500",
	domain.ImageBackdrop: "w1280",
	domain.ImageLogo:     "w500",
	domain.ImageStill:    "w300",
}

// imagePathPattern accepts only the path shape TMDB uses for the raster images
// the app serves (for example /9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg). Anything else is
// dropped, so a path can never add segments or query parameters to a URL, and
// SVG logos never reach the domain (spec §6).
var imagePathPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|jpeg|png)$`)

// ImageURL returns the CDN URL of an image path at the size of its kind. ok is
// false for an unknown kind or a path that is not a TMDB image path.
func ImageURL(kind domain.ImageKind, path string) (string, bool) {
	return imageURL(imageBase, kind, path)
}

func imageURL(base string, kind domain.ImageKind, path string) (string, bool) {
	size, known := imageSizes[kind]
	if !known || !imagePathPattern.MatchString(path) {
		return "", false
	}
	return base + size + path, true
}

// validPath returns path when it is a TMDB image path, else "".
func validPath(path string) string {
	if imagePathPattern.MatchString(path) {
		return path
	}
	return ""
}

// ImageSource downloads title images from TMDB's CDN. It implements
// application.ImageSource. The CDN is public, so it needs no credential.
type ImageSource struct {
	http *http.Client
	base string
}

var _ application.ImageSource = (*ImageSource)(nil)

// NewImageSource builds the source. hc may be nil (a client with a 30 s
// timeout); baseURL may be empty (the CDN of spec §6, tests point it elsewhere).
func NewImageSource(hc *http.Client, baseURL string) *ImageSource {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = imageBase
	}
	return &ImageSource{http: hc, base: baseURL}
}

// Fetch returns the body of the image at the size of its kind. The caller closes it.
// domain.ErrNotFound when the CDN does not have the image, domain.ErrUpstream for any other failure.
func (s *ImageSource) Fetch(ctx context.Context, ref domain.ImageRef) (io.ReadCloser, error) {
	u, ok := imageURL(s.base, ref.Kind, "/"+ref.File)
	if !ok {
		return nil, domain.ErrInvalidImage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: image request: %w", domain.ErrUpstream, err)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: image CDN unreachable: %w", domain.ErrUpstream, err)
	}
	switch res.StatusCode {
	case http.StatusOK:
		return res.Body, nil
	case http.StatusNotFound:
		_ = res.Body.Close()
		return nil, domain.ErrNotFound
	default:
		_ = res.Body.Close()
		return nil, fmt.Errorf("%w: image CDN returned %d", domain.ErrUpstream, res.StatusCode)
	}
}
