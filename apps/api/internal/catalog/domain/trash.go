// Package domain is the catalog model: the library's titles and files and what
// the trash keeps of them (spec RF-30). Everything here is pure: no I/O and no
// framework.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// TrashOrigin says where the files of a trash entry came from.
type TrashOrigin string

// Origins the trash records so far.
const (
	// TrashOriginUpload: the original of an archive that was extracted (RF-06).
	TrashOriginUpload TrashOrigin = "upload"
)

// TrashReason says why the files went to the trash.
type TrashReason string

// Reasons the trash records so far.
const (
	// TrashReasonExtracted: the archive was extracted, so its original is no longer needed (RF-06).
	TrashReasonExtracted TrashReason = "extracted"
	// TrashReasonFiltered: the filter discarded the file, a sample or something that is not video or subtitle (RF-07).
	TrashReasonFiltered TrashReason = "filtered"
)

// ErrInvalidTrash is returned for a trash entry whose values are not usable.
var ErrInvalidTrash = errors.New("invalid trash entry")

// TrashEntry is what went to the trash together: its files live in one folder of
// the trash, and this is their metadata (RF-30).
type TrashEntry struct {
	ID     int64
	Origin TrashOrigin
	Reason TrashReason
	// Dir is the entry's folder inside the trash: a random name, never a path.
	Dir       string
	TrashedAt time.Time
	// BatchID and RelativePath say where the file came from: the batch it was
	// dropped with and its path in that drop (RF-30). Empty for entries recorded
	// before they were kept.
	BatchID      string
	RelativePath string
	Files        []TrashFile
}

// Provenance is where a trashed file came from (RF-30).
type Provenance struct {
	// BatchID is the batch the file was dropped with.
	BatchID string
	// RelativePath is the file's path in the drop, with "/" separators, ending in its name.
	RelativePath string
}

// TrashFile is one file of a trash entry, with the name it had.
type TrashFile struct {
	// Name is the file name, as it was before it was trashed.
	Name string
	Size int64
}

// NewTrashEntry checks the values of a new trash entry with one file.
func NewTrashEntry(origin TrashOrigin, reason TrashReason, dir string, from Provenance, file TrashFile, now time.Time) (TrashEntry, error) {
	switch origin {
	case TrashOriginUpload:
	default:
		return TrashEntry{}, fmt.Errorf("%w: origin %q", ErrInvalidTrash, origin)
	}
	switch reason {
	case TrashReasonExtracted, TrashReasonFiltered:
	default:
		return TrashEntry{}, fmt.Errorf("%w: reason %q", ErrInvalidTrash, reason)
	}
	if !safeComponent(dir) {
		return TrashEntry{}, fmt.Errorf("%w: folder %q", ErrInvalidTrash, dir)
	}
	if !safeComponent(file.Name) {
		return TrashEntry{}, fmt.Errorf("%w: file name %q", ErrInvalidTrash, file.Name)
	}
	if file.Size < 0 {
		return TrashEntry{}, fmt.Errorf("%w: negative size", ErrInvalidTrash)
	}
	if !safeComponent(from.BatchID) {
		return TrashEntry{}, fmt.Errorf("%w: batch %q", ErrInvalidTrash, from.BatchID)
	}
	if !safeRelativePath(from.RelativePath) {
		return TrashEntry{}, fmt.Errorf("%w: relative path %q", ErrInvalidTrash, from.RelativePath)
	}
	return TrashEntry{
		Origin: origin, Reason: reason, Dir: dir, TrashedAt: now,
		BatchID: from.BatchID, RelativePath: from.RelativePath, Files: []TrashFile{file},
	}, nil
}

// safeComponent reports whether s is one path component that cannot leave its folder.
func safeComponent(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`) && !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 })
}

// safeRelativePath reports whether s is a path of safe components, joined with "/".
func safeRelativePath(s string) bool {
	if s == "" {
		return false
	}
	for part := range strings.SplitSeq(s, "/") {
		if !safeComponent(part) {
			return false
		}
	}
	return true
}
