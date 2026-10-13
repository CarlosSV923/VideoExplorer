package fileid

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

// Links implements application.LinkChecker with the file system: each part of a path below
// the library is looked at with Lstat, so a symbolic link is seen as itself and never followed.
type Links struct{}

var _ application.LinkChecker = Links{}

// NoLinks implements application.LinkChecker. A path that is not under root fails with an
// error of its own, before anything is looked at.
func (Links) NoLinks(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("fileid: the path is not under the library: " + path)
	}
	cur := root
	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil // the rest does not exist either, so it cannot be a link
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return application.ErrSymbolicLink
		}
	}
	return nil
}
