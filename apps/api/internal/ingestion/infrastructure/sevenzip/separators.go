package sevenzip

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// windowsSeparator is the folder separator of Windows. Some tools write it into the names of a zip's entries:
// PowerShell 5.1 (Compress-Archive) and .NET Framework do. On Linux 7zz takes it as a character of the name,
// so `Serie\Season 01\a.mkv` would be extracted as one file with that name (RF-10c). The entries of such a zip
// are moved to the folders their names describe after 7zz has written them, and the checks of the extraction
// (zip-slip, size and CRC32) are made on the normalized paths.
const windowsSeparator = `\`

// separatorMove is one entry of a zip whose name uses windowsSeparator.
type separatorMove struct {
	// from is the name 7zz wrote the entry under, relative to the extraction folder.
	from string
	// to is the normalized path, relative to the extraction folder, with "/" separators.
	to  string
	dir bool
}

// isZip reports whether the archive is a zip, by its first bytes (RF-04). Only zips are normalized.
func isZip(archive string) (bool, error) {
	f, err := os.Open(archive) //nolint:gosec // the path is the staged archive, built by the app
	if err != nil {
		return false, err
	}
	defer f.Close()
	header := make([]byte, 8)
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	format, ok := domain.DetectArchive(header[:n])
	return ok && format == domain.ArchiveZip, nil
}

// planSeparators returns the moves that put the entries whose names use the Windows separator where those names
// say (RF-10c). An entry with the separator whose normalized path leaves the extraction folder is refused before
// anything is extracted (ErrUnsafeEntry), and so is a name that another entry would overwrite (ErrCorrupt).
// Entries without the separator are not moved.
func planSeparators(entries []application.ArchiveEntry) ([]separatorMove, error) {
	seen := make(map[string]string, len(entries)) // normalized path -> name in the archive
	var moves []separatorMove
	for _, e := range entries {
		norm := e.Path
		if strings.Contains(e.Path, windowsSeparator) {
			norm = strings.TrimSuffix(strings.ReplaceAll(e.Path, windowsSeparator, "/"), "/")
		}
		if !e.IsDir {
			if prev, dup := seen[norm]; dup && prev != e.Path {
				return nil, fmt.Errorf("%w: %q and %q are the same path", application.ErrCorrupt, prev, e.Path)
			}
			seen[norm] = e.Path
		}
		if norm == e.Path {
			continue
		}
		if !safeRelPath(norm) {
			return nil, fmt.Errorf("%w: %q leaves the extraction folder", application.ErrUnsafeEntry, e.Path)
		}
		moves = append(moves, separatorMove{from: e.Path, to: norm, dir: e.IsDir})
	}
	return moves, nil
}

// safeRelPath reports whether a normalized path stays inside the folder it is relative to: it is relative, has
// no drive letter, and no empty, "." or ".." component.
func safeRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return false
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// normalizeSeparators moves what 7zz extracted under the names of the moves to the folders those names describe
// (RF-10c). It runs after the extraction and before the verification, which checks the normalized paths. A target
// that already exists is refused: two entries cannot share one path.
func normalizeSeparators(dest string, moves []separatorMove) error {
	for _, m := range moves {
		from := filepath.Join(dest, m.from) // on Linux the separator is a character of the name
		to := filepath.Join(dest, filepath.FromSlash(m.to))
		if m.dir {
			if err := os.MkdirAll(to, 0o750); err != nil {
				return fmt.Errorf("%w: folder %s: %w", application.ErrCorrupt, m.to, err)
			}
			if err := os.Remove(from); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%w: folder %s: %w", application.ErrCorrupt, m.from, err)
			}
			continue
		}
		if _, err := os.Lstat(to); err == nil {
			return fmt.Errorf("%w: %s is already extracted", application.ErrCorrupt, m.to)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			return fmt.Errorf("%w: folder of %s: %w", application.ErrCorrupt, m.to, err)
		}
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("%w: %s: %w", application.ErrCorrupt, m.from, err)
		}
	}
	return nil
}
