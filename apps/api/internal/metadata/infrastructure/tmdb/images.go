package tmdb

import (
	"regexp"

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

// imagePathPattern accepts only the path shape TMDB uses (for example
// /9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg). Anything else is dropped, so a path can
// never add segments or query parameters to a URL.
var imagePathPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|jpeg|png)$`)

// ImageURL returns the CDN URL of an image path at the size of its kind. ok is
// false for an unknown kind or a path that is not a TMDB image path.
func ImageURL(kind domain.ImageKind, path string) (string, bool) {
	size, known := imageSizes[kind]
	if !known || !imagePathPattern.MatchString(path) {
		return "", false
	}
	return imageBase + size + path, true
}

// validPath returns path when it is a TMDB image path, else "".
func validPath(path string) string {
	if imagePathPattern.MatchString(path) {
		return path
	}
	return ""
}
