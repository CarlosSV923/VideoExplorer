package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Errors returned while preparing a download. The HTTP layer maps unavailable
// files, including symbolic links, to 404 so filesystem details do not escape.
var (
	ErrDownloadUnavailable = errors.New("download file is unavailable")
)

// DownloadFiles is the safe read side of the catalog filesystem. The adapter
// validates that every path is inside the library and does not pass through a
// symbolic link before opening or statting it.
type DownloadFiles interface {
	LibraryPath(elem ...string) string
	UnassignedPath(elem ...string) string
	Info(path string) (FileInfo, error)
	Open(path string) (io.ReadSeekCloser, error)
}

// FileInfo is the stable metadata needed by ServeContent and zipstream.
type FileInfo struct {
	Size     int64
	Modified time.Time
}

// Download describes one raw file or a streamed store zip.
type Download struct {
	Name    string
	File    *DownloadFile
	Entries []DownloadEntry
}

// DownloadFile is one raw file download. Path is absolute but is created only
// by the server from a registry or section record.
type DownloadFile struct {
	Path     string
	Size     int64
	Modified time.Time
}

// DownloadEntry is one file inside a streamed zip.
type DownloadEntry struct {
	Name     string
	Path     string
	Size     int64
	Modified time.Time
}

// LibraryDownloads prepares downloads from the live library registry.
type LibraryDownloads struct {
	repo  LibraryRepository
	files DownloadFiles
}

// NewLibraryDownloads builds the library download read side.
func NewLibraryDownloads(repo LibraryRepository, files DownloadFiles) *LibraryDownloads {
	return &LibraryDownloads{repo: repo, files: files}
}

// Files exposes the already validated filesystem port to the HTTP streaming
// adapter; callers never provide a path to it.
func (d *LibraryDownloads) Files() DownloadFiles { return d.files }

// File prepares a live video or subtitle selected by the title and file ids.
func (d *LibraryDownloads) File(ctx context.Context, titleID, fileID int64) (Download, error) {
	title, files, err := d.liveTitle(ctx, titleID)
	if err != nil {
		return Download{}, err
	}
	for _, f := range files {
		if f.ID == fileID && (f.Kind == domain.FileVideo || f.Kind == domain.FileSubtitle) {
			file, err := d.file(title, f.Rel)
			if err != nil {
				return Download{}, err
			}
			return Download{Name: path.Base(f.Rel), File: file}, nil
		}
	}
	return Download{}, ErrLibraryFileNotFound
}

// Title prepares the complete live title as a store zip.
func (d *LibraryDownloads) Title(ctx context.Context, titleID int64) (Download, error) {
	title, files, err := d.liveTitle(ctx, titleID)
	if err != nil {
		return Download{}, err
	}
	return d.zip(title, files, titleFolderName(title.Folder)+".zip")
}

// Season prepares one season of a live series as a store zip.
func (d *LibraryDownloads) Season(ctx context.Context, titleID int64, season int) (Download, error) {
	title, files, err := d.liveTitle(ctx, titleID)
	if err != nil {
		return Download{}, err
	}
	if title.Kind != domain.KindSeries {
		return Download{}, ErrLibraryFileNotFound
	}
	var selected []domain.LibraryFile
	videoRels := map[string]bool{}
	for _, f := range files {
		if f.Kind == domain.FileVideo && f.Place.Episode && f.Place.Season == season {
			selected = append(selected, f)
			videoRels[strings.ToLower(f.Rel)] = true
		}
	}
	if len(selected) == 0 {
		return Download{}, ErrLibraryFileNotFound
	}
	for _, f := range files {
		if f.Kind == domain.FileSubtitle && videoRels[strings.ToLower(f.VideoRel)] {
			selected = append(selected, f)
		}
	}
	name := fmt.Sprintf("%s - Season %02d.zip", titleFolderName(title.Folder), season)
	return d.zip(title, selected, name)
}

func (d *LibraryDownloads) liveTitle(ctx context.Context, id int64) (domain.LibraryTitle, []domain.LibraryFile, error) {
	titles, err := d.repo.Titles(ctx)
	if err != nil {
		return domain.LibraryTitle{}, nil, err
	}
	var title domain.LibraryTitle
	for _, candidate := range titles {
		if candidate.ID == id {
			title = candidate
			break
		}
	}
	if title.ID == 0 {
		return domain.LibraryTitle{}, nil, ErrTitleNotFound
	}
	files, err := d.repo.Files(ctx, id)
	if err != nil {
		return domain.LibraryTitle{}, nil, err
	}
	live := make([]domain.LibraryFile, 0, len(files))
	for _, f := range files {
		if f.Live() && (f.Kind == domain.FileVideo || f.Kind == domain.FileSubtitle) {
			live = append(live, f)
		}
	}
	if len(live) == 0 {
		return domain.LibraryTitle{}, nil, ErrTitleNotFound
	}
	return title, live, nil
}

func (d *LibraryDownloads) zip(title domain.LibraryTitle, files []domain.LibraryFile, name string) (Download, error) {
	slices.SortFunc(files, func(a, b domain.LibraryFile) int {
		return strings.Compare(strings.ToLower(a.Rel), strings.ToLower(b.Rel))
	})
	out := Download{Name: name, Entries: make([]DownloadEntry, 0, len(files))}
	root := titleFolderName(title.Folder)
	for _, f := range files {
		info, err := d.info(title.Folder, f.Rel)
		if err != nil {
			return Download{}, err
		}
		rel := strings.TrimPrefix(f.Rel, strings.TrimSuffix(title.Folder, "/")+"/")
		out.Entries = append(out.Entries, DownloadEntry{
			Name: path.Join(root, rel), Path: d.files.LibraryPath(filepath.FromSlash(f.Rel)), Size: info.Size, Modified: info.Modified,
		})
	}
	return out, nil
}

func (d *LibraryDownloads) file(title domain.LibraryTitle, rel string) (*DownloadFile, error) {
	info, err := d.info(title.Folder, rel)
	if err != nil {
		return nil, err
	}
	return &DownloadFile{Path: d.files.LibraryPath(filepath.FromSlash(rel)), Size: info.Size, Modified: info.Modified}, nil
}

func (d *LibraryDownloads) info(_ string, rel string) (FileInfo, error) {
	info, err := d.files.Info(d.files.LibraryPath(filepath.FromSlash(rel)))
	if err != nil {
		return FileInfo{}, fmt.Errorf("%w: %s: %w", ErrDownloadUnavailable, rel, err)
	}
	return info, nil
}

func titleFolderName(folder string) string { return path.Base(strings.TrimSuffix(folder, "/")) }
