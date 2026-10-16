// Package zipstream writes uncompressed (store) zip archives while they are
// sent, and computes their exact size beforehand so a download can announce
// its Content-Length. Video files barely compress, and compressing would cost
// CPU on the NAS.
package zipstream

import (
	"archive/zip"
	"fmt"
	"io"
	"math"
	"time"
)

// Entry is one file of the archive.
type Entry struct {
	// Name is the path inside the archive ("/" separators).
	Name     string
	Size     int64
	Modified time.Time
	// Open returns the file contents; it is called while writing.
	Open func() (io.ReadCloser, error)
}

// Sizes of the zip records written by archive/zip.
const (
	localHeaderLen      = 30
	centralHeaderLen    = 46
	dataDescriptorLen   = 16
	dataDescriptor64Len = 24
	timestampExtraLen   = 9
	zip64ExtraHeaderLen = 4
	endLen              = 22
	end64Len            = 56
	end64LocatorLen     = 20
	uint32max           = math.MaxUint32
	uint16max           = math.MaxUint16
)

// Size is the exact number of bytes Write produces for entries. It mirrors
// archive/zip's layout for store entries with a data descriptor: a central
// zip64 extra with each size and offset that needs 64 bits, a 64-bit data
// descriptor for large entries, and the zip64 end records when needed.
func Size(entries []Entry) int64 {
	var offset, central int64
	usedZip64 := false
	for _, e := range entries {
		name := int64(len(e.Name))
		extra := int64(0)
		if !e.Modified.IsZero() {
			extra = timestampExtraLen
		}
		descriptor := int64(dataDescriptorLen)
		if e.Size > uint32max {
			descriptor = dataDescriptor64Len
		}
		centralExtra := extra
		fields := int64(0)
		if e.Size >= uint32max {
			fields += 2
		}
		if offset >= uint32max {
			fields++
		}
		if fields > 0 {
			usedZip64 = true
			centralExtra += zip64ExtraHeaderLen + 8*fields
		}
		central += centralHeaderLen + name + centralExtra
		offset += localHeaderLen + name + extra + e.Size + descriptor
	}
	total := offset + central + endLen
	if usedZip64 || len(entries) >= uint16max || central >= uint32max || offset >= uint32max {
		total += end64Len + end64LocatorLen
	}
	return total
}

// Write streams entries as a store zip to w. A file whose size changed since
// the entry was built fails the write, since Size was already announced.
func Write(w io.Writer, entries []Entry) error {
	zw := zip.NewWriter(w)
	for _, e := range entries {
		dst, err := zw.CreateHeader(&zip.FileHeader{Name: e.Name, Method: zip.Store, Modified: e.Modified})
		if err != nil {
			return err
		}
		if err := copyEntry(dst, e); err != nil {
			return err
		}
	}
	return zw.Close()
}

func copyEntry(dst io.Writer, e Entry) error {
	src, err := e.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	n, err := io.Copy(dst, io.LimitReader(src, e.Size))
	if err != nil {
		return err
	}
	if n != e.Size {
		return fmt.Errorf("zipstream: %s has %d bytes, expected %d", e.Name, n, e.Size)
	}
	return nil
}
