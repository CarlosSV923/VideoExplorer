// Package partialhash computes the partial hash of a file (RF-24). It reads only the ranges
// of domain.PartialHashRanges, through one small buffer that every range reuses, so the
// file is never read whole and never held in memory (RNF-01).
package partialhash

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// bufferSize is the size of the buffer the reads of one hash go through.
const bufferSize = 64 << 10

// Hasher implements application.PartialHasher with the files of the local file system.
type Hasher struct{}

var _ application.PartialHasher = Hasher{}

// PartialHash implements application.PartialHasher. The file is opened and its size is
// the one it has now; a path that is not a regular file fails.
func (Hasher) PartialHash(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the path is built by the app from IDs (RNF-02); only its ranges are read
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("partialhash: not a regular file: %s", info.Mode().Type())
	}
	return hashRanges(ctx, f, info.Size(), make([]byte, bufferSize))
}

// hashRanges returns the partial hash of a file of size bytes read through r: its ranges
// in order, through buf. It fails when r ends before a range does (the file shrank) and
// when ctx ends.
func hashRanges(ctx context.Context, r io.ReaderAt, size int64, buf []byte) (string, error) {
	h := domain.NewPartialDigest(size)
	for _, rg := range domain.PartialHashRanges(size) {
		if err := copyRange(ctx, h, r, rg, buf); err != nil {
			return "", err
		}
	}
	return domain.FormatPartialHash(h.Sum(nil)), nil
}

// copyRange writes the bytes of rg into w, read through buf. ctx is checked before each read.
func copyRange(ctx context.Context, w io.Writer, r io.ReaderAt, rg domain.ByteRange, buf []byte) error {
	end := rg.Offset + rg.Length
	for off := rg.Offset; off < end; {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := min(int64(len(buf)), end-off)
		n, err := r.ReadAt(buf[:want], off)
		_, _ = w.Write(buf[:n]) // a hash never fails to take bytes
		off += int64(n)
		if int64(n) < want {
			if err == nil || errors.Is(err, io.EOF) {
				return fmt.Errorf("partialhash: file ended at %d before its range ended: %w", off, io.ErrUnexpectedEOF)
			}
			return err
		}
	}
	return nil
}
