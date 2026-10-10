// Package imagecache keeps the title images on disk, under
// DATA_PATH/cache/images/<kind>/<file>. An image is downloaded from TMDB's CDN
// the first time it is asked for and then served locally; the browser never
// contacts TMDB (spec §6, RNF-10).
package imagecache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// maxImageBytes bounds one download. The largest size of spec §6 (backdrop
// w1280) is far below it; a bigger body is not a title image.
const maxImageBytes = 10 << 20

// tempPrefix names the files being downloaded. They are written next to their
// final place and renamed when complete, so a reader never sees a partial image.
const tempPrefix = ".download-"

// staleTempAge is how old an unfinished download must be before Prune removes it.
const staleTempAge = time.Hour

// sniffLen is how many bytes http.DetectContentType reads.
const sniffLen = 512

// Cache implements application.ImageStore on a folder of DATA_PATH.
type Cache struct {
	dir    string
	source application.ImageSource // nil when TMDB is not configured
	now    func() time.Time
}

var _ application.ImageStore = (*Cache)(nil)

// New builds a cache rooted at dir (DATA_PATH/cache/images). source is nil
// when TMDB is not configured: then only the images already on disk are served.
func New(dir string, source application.ImageSource) *Cache {
	return &Cache{dir: dir, source: source, now: time.Now}
}

// path is the file of ref. ref is validated by domain.NewImageRef, so the
// result is always inside dir.
func (c *Cache) path(ref domain.ImageRef) string {
	return filepath.Join(c.dir, string(ref.Kind), ref.File)
}

// Open returns the cached image of ref, downloading it when it is missing.
func (c *Cache) Open(ctx context.Context, ref domain.ImageRef) (application.StoredImage, error) {
	img, err := openFile(c.path(ref))
	if err == nil {
		return img, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return application.StoredImage{}, fmt.Errorf("image cache: %w", err)
	}
	if c.source == nil {
		return application.StoredImage{}, domain.ErrNotConfigured
	}
	if err := c.download(ctx, ref); err != nil {
		return application.StoredImage{}, err
	}
	return openFile(c.path(ref))
}

func openFile(p string) (application.StoredImage, error) {
	f, err := os.Open(p) //nolint:gosec // path built from a validated ImageRef
	if err != nil {
		return application.StoredImage{}, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return application.StoredImage{}, err
	}
	return application.StoredImage{File: f, ModTime: info.ModTime()}, nil
}

// download streams the image from the source into a temporary file, checks
// its size and its type, and renames it into place. Any failure removes the
// temporary file, so nothing partial is ever visible under the final name.
func (c *Cache) download(ctx context.Context, ref domain.ImageRef) error {
	body, err := c.source.Fetch(ctx, ref)
	if err != nil {
		return err
	}
	defer body.Close()

	dest := c.path(ref)
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("image cache: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("image cache: %w", err)
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmp.Name())
		}
	}()

	n, copyErr := io.Copy(tmp, io.LimitReader(body, maxImageBytes+1))
	closeErr := tmp.Close()
	switch {
	case copyErr != nil:
		return errors.Join(domain.ErrUpstream, fmt.Errorf("image download: %w", copyErr))
	case closeErr != nil:
		return fmt.Errorf("image cache: %w", closeErr)
	case n > maxImageBytes:
		return fmt.Errorf("%w: image larger than %d bytes", domain.ErrUpstream, maxImageBytes)
	}

	if err := checkType(tmp.Name(), ref.ContentType()); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("image cache: %w", err)
	}
	renamed = true
	return nil
}

// checkType sniffs the first bytes of a downloaded file and reports an error
// unless they are the type the file name promises. The content decides, not
// the header TMDB sends.
func checkType(path, want string) error {
	f, err := os.Open(path) //nolint:gosec // temporary file created by download
	if err != nil {
		return fmt.Errorf("image cache: %w", err)
	}
	defer f.Close()
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("image cache: %w", err)
	}
	if got := http.DetectContentType(head[:n]); got != want {
		return fmt.Errorf("%w: TMDB sent %q for an image that must be %q", domain.ErrUpstream, got, want)
	}
	return nil
}

// Prune deletes the cached images that are not in keep. It also removes
// unfinished downloads older than an hour, which a crash can leave behind.
// Recent temporary files are left alone: they may belong to a download in progress.
func (c *Cache) Prune(ctx context.Context, keep []domain.ImageRef) (int, error) {
	kept := make(map[string]bool, len(keep))
	for _, ref := range keep {
		kept[c.path(ref)] = true
	}
	removed := 0
	for _, kind := range domain.ImageKinds() {
		n, err := c.pruneKind(ctx, filepath.Join(c.dir, string(kind)), kept)
		removed += n
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}

func (c *Cache) pruneKind(ctx context.Context, dir string, kept map[string]bool) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("image cache: %w", err)
	}
	removed := 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if strings.HasPrefix(e.Name(), tempPrefix) {
			if !c.staleTemp(e) {
				continue
			}
		} else if kept[p] {
			continue
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, fmt.Errorf("image cache: %w", err)
		}
		removed++
	}
	return removed, nil
}

// staleTemp reports whether a temporary download has been idle for staleTempAge.
func (c *Cache) staleTemp(e fs.DirEntry) bool {
	info, err := e.Info()
	if err != nil {
		return false
	}
	return c.now().Sub(info.ModTime()) > staleTempAge
}
