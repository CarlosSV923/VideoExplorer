package application

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Errors an Extractor returns. The extraction maps them to the reasons of RF-10c.
var (
	// ErrPasswordRequired means the archive is encrypted and no password was given.
	ErrPasswordRequired = errors.New("archive password required")
	// ErrWrongPassword means the password given does not open the archive.
	ErrWrongPassword = errors.New("wrong archive password")
	// ErrCorrupt means 7-Zip failed on the archive, or an extracted file is missing.
	ErrCorrupt = errors.New("archive is corrupt")
	// ErrMismatch means an extracted file differs in size or CRC32 from the archive index (RF-05).
	ErrMismatch = errors.New("extracted file does not match the archive index")
	// ErrUnsafeEntry means an entry would leave the extraction folder, or is a link or a special file.
	ErrUnsafeEntry = errors.New("unsafe entry in archive")
	// ErrNoSpace means the library has no room to extract: checked before extracting
	// (RF-05) or reported by the tool when the disk fills up while it writes.
	ErrNoSpace = errors.New("not enough free space to extract")
)

// ArchiveEntry is one entry of an archive's index.
type ArchiveEntry struct {
	Path      string
	Size      int64
	CRC       string // upper-case hex CRC32, "" when the format has none
	IsDir     bool
	Symlink   bool
	Encrypted bool
}

// Listing is an archive's index.
type Listing struct {
	Entries []ArchiveEntry
}

// TotalSize is the uncompressed size of every file in the archive.
func (l Listing) TotalSize() int64 {
	var n int64
	for _, e := range l.Entries {
		if !e.IsDir {
			n += e.Size
		}
	}
	return n
}

// Encrypted reports whether any entry needs a password.
func (l Listing) Encrypted() bool {
	for _, e := range l.Entries {
		if e.Encrypted {
			return true
		}
	}
	return false
}

// Extractor lists and extracts archives with the official 7-Zip (RNF-03a). The
// bytes never enter the process: the tool reads and writes them.
type Extractor interface {
	// List returns the index of the archive. An archive whose index is encrypted
	// fails with ErrPasswordRequired (no password) or ErrWrongPassword.
	List(ctx context.Context, archive, password string) (Listing, error)
	// Extract unpacks the archive into dest and checks every file against the
	// listing (size and CRC32). The warning is not an error: attributes that the
	// file system does not allow to set (RF-05).
	Extract(ctx context.Context, archive, dest, password string, listing Listing, progress func(pct int)) (warning string, err error)
}

// Workspace is where the archives wait and where they are extracted: the
// staging area, on the same file system as the library (RNF-04).
type Workspace interface {
	// ArchivePath is the file of an upload, which is the archive while it waits.
	ArchivePath(id domain.JobID) string
	// ArchiveExists reports whether the upload's file is still staged.
	ArchiveExists(id domain.JobID) bool
	// VolumesDir is the folder where the volumes of one claimed set gather (RF-03).
	VolumesDir(group string) string
	// VolumePath is the file of a volume inside its set's folder.
	VolumePath(group, name string) string
	// VolumeExists reports whether a volume is in its set's folder.
	VolumeExists(group, name string) bool
	// ClaimVolume moves the staged file of a volume into its set's folder, under its
	// own name. A volume already there is left as it is.
	ClaimVolume(id domain.JobID, group, name string) error
	// RemoveVolumes deletes a set's folder and the volumes in it, if any.
	RemoveVolumes(group string) error
	// ExtractionDir is the job's folder for the extracted files.
	ExtractionDir(id domain.JobID) string
	// ResetExtraction empties (or creates) the job's extraction folder.
	ResetExtraction(id domain.JobID) error
	// RemoveExtraction deletes the job's extraction folder, if any.
	RemoveExtraction(id domain.JobID) error
	// CheckExtraction rejects anything extracted that is not a regular file or a
	// folder (links could point outside the library).
	CheckExtraction(id domain.JobID) error
	// FreeSpace is the free space of the file system that holds the staging area.
	FreeSpace() (uint64, error)
	// ReadHeader reads the first n bytes of a file (fewer at the end of the file).
	ReadHeader(path string, n int) ([]byte, error)
	// ExtractionExists reports whether the job (or the set led by it) has an extraction
	// folder: only archives have one (RF-07).
	ExtractionExists(id domain.JobID) bool
	// ExtractedFiles lists the regular files of the extraction folder, with their paths
	// relative to it and "/" separators (RF-07).
	ExtractedFiles(id domain.JobID) ([]ExtractedFile, error)
	// ExtractedPath is the file at a relative path of the extraction folder.
	ExtractedPath(id domain.JobID, rel string) string
	// FileExists reports whether a regular file is at path, a path the service built.
	FileExists(path string) bool
}

// ExtractedFile is a file of an extraction folder.
type ExtractedFile struct {
	// Path is relative to the extraction folder, with "/" separators.
	Path string
	Size int64
}

// Trash receives the originals of finished archives (RF-06). The catalog context
// implements it, with the trash of RF-30.
type Trash interface {
	// Put moves the file at Source into the trash as one entry, with its metadata.
	Put(ctx context.Context, p TrashPut) error
}

// Origin and reason values that ingestion sends to the trash.
const (
	TrashOriginUpload = "upload"
	// TrashOriginSamba is the origin of the copy of a failed Samba file (RF-27a).
	TrashOriginSamba     = "samba"
	TrashReasonExtracted = "extracted"
	TrashReasonFiltered  = "filtered"
	// TrashReasonRemoved is the reason of a copy of Samba that failed without Reintentar (RF-27a).
	TrashReasonRemoved = "removed"
)

// TrashPut is one file that goes to the trash.
type TrashPut struct {
	// Origin says where the file came from (TrashOriginUpload).
	Origin string
	// Reason says why it goes to the trash (TrashReasonExtracted).
	Reason string
	// Name is the file's name, kept in the trash.
	Name string
	// BatchID is the batch the file was dropped with, and RelativePath its path in
	// that drop, ending in its name (RF-30).
	BatchID      string
	RelativePath string
	// Source is the file's current path, which is inside the library.
	Source string
	Size   int64
}
