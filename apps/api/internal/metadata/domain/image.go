package domain

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidImage means an image request names no valid kind or file (spec §6, §9).
var ErrInvalidImage = errors.New("invalid image")

// imageFilePattern is the file name of a TMDB image without its leading slash:
// letters, digits, "_" and "-", then a raster extension in lower case. It has
// no separator and no dot segment, so the name can be joined to a folder safely.
var imageFilePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}\.(jpe?g|png)$`)

// ImageKinds lists every kind of image, in the order of spec §6.
func ImageKinds() []ImageKind {
	return []ImageKind{ImagePoster, ImageBackdrop, ImageLogo, ImageStill}
}

// ParseImageKind accepts one of the kinds of ImageKinds and nothing else.
func ParseImageKind(s string) (ImageKind, error) {
	k := ImageKind(s)
	for _, known := range ImageKinds() {
		if k == known {
			return k, nil
		}
	}
	return "", ErrInvalidImage
}

// ImageRef identifies one image of TMDB: its kind (which sets the size) and its
// file name. Both end up in a file path and a URL, so they are only built by NewImageRef.
type ImageRef struct {
	Kind ImageKind
	File string
}

// NewImageRef validates the kind and the file name of an image.
func NewImageRef(kind, file string) (ImageRef, error) {
	k, err := ParseImageKind(kind)
	if err != nil {
		return ImageRef{}, err
	}
	if !imageFilePattern.MatchString(file) {
		return ImageRef{}, ErrInvalidImage
	}
	return ImageRef{Kind: k, File: file}, nil
}

// isServablePath reports whether a TMDB image path (for example /abc.png) is one
// the app serves: a JPEG or a PNG (spec §6). SVG logos are not, so they are never chosen.
func isServablePath(path string) bool {
	return strings.HasPrefix(path, "/") && imageFilePattern.MatchString(path[1:])
}

// ContentType is the media type the file of ref must have: PNG for .png, JPEG otherwise.
func (r ImageRef) ContentType() string {
	if strings.HasSuffix(r.File, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}
