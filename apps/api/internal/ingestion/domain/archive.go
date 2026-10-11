package domain

import "bytes"

// ArchiveFormat is a compressed container the pipeline extracts (RF-04).
type ArchiveFormat string

// Supported archive formats.
const (
	ArchiveZip ArchiveFormat = "zip"
	Archive7z  ArchiveFormat = "7z"
	ArchiveRar ArchiveFormat = "rar"
)

// DetectArchive recognizes an archive by its magic bytes, never by its
// extension: a file renamed to .zip that is a video is still a video. It needs
// the first 8 bytes of the file.
func DetectArchive(header []byte) (ArchiveFormat, bool) {
	switch {
	case bytes.HasPrefix(header, []byte("PK\x03\x04")), bytes.HasPrefix(header, []byte("PK\x05\x06")):
		return ArchiveZip, true
	case bytes.HasPrefix(header, []byte("7z\xBC\xAF\x27\x1C")):
		return Archive7z, true
	case bytes.HasPrefix(header, []byte("Rar!\x1A\x07\x00")), bytes.HasPrefix(header, []byte("Rar!\x1A\x07\x01\x00")):
		return ArchiveRar, true
	}
	return "", false
}
