// Package domain holds the ingestion rules that need no I/O: what a valid
// upload declares and whether the library has room for it.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// FreeSpaceMargin is kept free on top of every upload's size (spec RF-05).
const FreeSpaceMargin uint64 = 1 << 30 // 1 GiB

const (
	maxComponentBytes = 255
	maxRelativeBytes  = 4096
)

var (
	// ErrInvalidMetadata means the form data an upload declares is not usable.
	ErrInvalidMetadata = errors.New("invalid upload metadata")
	// ErrInsufficientSpace means the library cannot take the upload with its margin.
	ErrInsufficientSpace = errors.New("the library has not enough free space")
)

// batchIDPattern is the shape of a batch id: short and URL-safe, so it can be
// a single path component or a query value. The server makes the ids (RF-03).
var batchIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// UploadMeta is the form data an upload declares when it starts (RF-01, RF-01a, RF-03).
type UploadMeta struct {
	// FileName is the name of the file, without folders.
	FileName string
	// RelativePath is the path of the file inside the folder the browser dropped
	// (RF-01a), ending with FileName. Empty for a file dropped on its own.
	RelativePath string
	// BatchID groups the files dropped together (RF-03). Empty when there is none.
	BatchID string
}

// NewUploadMeta checks the declared values and returns them sanitized: names
// are trimmed. Anything that could leave the upload's folder is rejected:
// separators, '.' and '..', control characters, names longer than 255 bytes
// and relative paths that are absolute or that do not end with the file name.
func NewUploadMeta(fileName, relativePath, batchID string) (UploadMeta, error) {
	name, err := cleanComponent(fileName)
	if err != nil {
		return UploadMeta{}, fmt.Errorf("%w: filename %w", ErrInvalidMetadata, err)
	}
	meta := UploadMeta{FileName: name}

	if relativePath != "" {
		rel, err := cleanRelativePath(relativePath, name)
		if err != nil {
			return UploadMeta{}, fmt.Errorf("%w: relativePath %w", ErrInvalidMetadata, err)
		}
		meta.RelativePath = rel
	}

	if batchID != "" {
		if !batchIDPattern.MatchString(batchID) {
			return UploadMeta{}, fmt.Errorf("%w: batchId must be 1 to 64 letters, digits, '-' or '_'", ErrInvalidMetadata)
		}
		meta.BatchID = batchID
	}
	return meta, nil
}

// cleanComponent trims one path component and rejects anything unsafe in it.
func cleanComponent(s string) (string, error) {
	name := strings.TrimSpace(s)
	switch {
	case name == "":
		return "", errors.New("is empty")
	case name == "." || name == "..":
		return "", errors.New("cannot be '.' or '..'")
	case len(name) > maxComponentBytes:
		return "", fmt.Errorf("is longer than %d bytes", maxComponentBytes)
	case !utf8.ValidString(name):
		return "", errors.New("is not valid UTF-8")
	case strings.ContainsAny(name, `/\`):
		return "", errors.New("cannot contain '/' or '\\'")
	case strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return "", errors.New("cannot contain control characters")
	}
	return name, nil
}

// cleanRelativePath checks every component of a path made of folders and
// file name. An empty component (a leading slash, a double slash) is refused,
// which is how absolute paths are caught.
func cleanRelativePath(p, fileName string) (string, error) {
	if len(p) > maxRelativeBytes {
		return "", fmt.Errorf("is longer than %d bytes", maxRelativeBytes)
	}
	parts := strings.Split(p, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		name, err := cleanComponent(part)
		if err != nil {
			return "", err
		}
		cleaned = append(cleaned, name)
	}
	if cleaned[len(cleaned)-1] != fileName {
		return "", errors.New("must end with the file name")
	}
	return strings.Join(cleaned, "/"), nil
}

// CheckFreeSpace reports whether the library can take an upload of size bytes
// with FreeSpaceMargin left over (RF-05, RF-09a). free is the space available
// to the process on the library's file system; pending is what the uploads in
// progress still have to receive, which is not free yet and is reserved for them.
func CheckFreeSpace(size int64, free, pending uint64) error {
	if size < 0 {
		return fmt.Errorf("%w: negative size", ErrInvalidMetadata)
	}
	var available uint64
	if free > pending {
		available = free - pending
	}
	need := uint64(size) + FreeSpaceMargin //nolint:gosec // size is non-negative, checked above
	if need > available {
		return fmt.Errorf("%w: need %d bytes including the %d GiB margin, %d available after reserving %d bytes of uploads in progress",
			ErrInsufficientSpace, need, FreeSpaceMargin>>30, available, pending)
	}
	return nil
}

// PendingBytes is how many bytes an upload of size bytes still has to receive
// when offset of them arrived. It is never negative.
func PendingBytes(size, offset int64) int64 {
	if offset >= size {
		return 0
	}
	return size - offset
}
