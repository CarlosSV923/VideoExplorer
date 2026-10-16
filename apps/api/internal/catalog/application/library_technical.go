package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// ErrLibraryFileNotFound means there is no live video with that id in the title.
var ErrLibraryFileNotFound = errors.New("the video is not in the library title")

// LibraryPath is the catalog's narrow port for building an absolute path from a
// registered relative path. The caller never supplies a path.
type LibraryPath interface {
	LibraryPath(elem ...string) string
}

// TechnicalProber is the catalog port for ffprobe. The composition root adapts
// ingestion's cached prober to these catalog-owned values.
type TechnicalProber interface {
	Probe(ctx context.Context, path string) (TechnicalMedia, error)
}

// TechnicalMedia is the technical information returned by the catalog's probe port.
type TechnicalMedia struct {
	Duration  time.Duration
	Video     TechnicalVideo
	Audio     []TechnicalAudio
	Subtitles []TechnicalSubtitle
}

// TechnicalVideo is the main video stream of a probed file.
type TechnicalVideo struct {
	Width, Height int
	HDR           bool
	Codec         string
}

// TechnicalAudio is one audio stream of a probed file.
type TechnicalAudio struct {
	Codec    string
	Channels int
	Language string
	Title    string
	Default  bool
}

// TechnicalSubtitle is one internal subtitle stream of a probed file.
type TechnicalSubtitle struct {
	Codec    string
	Language string
	Title    string
	Forced   bool
	SDH      bool
	Default  bool
}

// TechnicalFile is the technical view of one live video in a library title.
type TechnicalFile struct {
	Path      string
	Size      int64
	Readable  bool
	Duration  time.Duration
	Video     TechnicalVideo
	Audio     []TechnicalAudio
	Subtitles []TechnicalSubtitle
}

// LibraryTechnical reads the technical information of registered library videos.
type LibraryTechnical struct {
	repo   LibraryRepository
	paths  LibraryPath
	prober TechnicalProber
	log    *slog.Logger
}

// NewLibraryTechnical builds the technical read side. log may be nil.
func NewLibraryTechnical(repo LibraryRepository, paths LibraryPath, prober TechnicalProber, log *slog.Logger) *LibraryTechnical {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &LibraryTechnical{repo: repo, paths: paths, prober: prober, log: log}
}

// File returns a live video only when both ids identify the same title and file.
// A probe failure is deliberately represented as an unreadable, otherwise useful
// response: its registered path and size remain available to the title page.
func (t *LibraryTechnical) File(ctx context.Context, titleID, fileID int64) (TechnicalFile, error) {
	live, _, err := classifyTitles(ctx, t.repo)
	if err != nil {
		return TechnicalFile{}, err
	}
	if !containsTitle(live, titleID) {
		return TechnicalFile{}, ErrTitleNotFound
	}

	files, err := t.repo.Files(ctx, titleID)
	if err != nil {
		return TechnicalFile{}, err
	}
	for _, file := range files {
		if file.ID != fileID || file.TitleID != titleID || file.Kind != domain.FileVideo || !file.Live() {
			continue
		}
		out := TechnicalFile{
			Path:      file.Rel,
			Size:      file.Size,
			Audio:     []TechnicalAudio{},
			Subtitles: []TechnicalSubtitle{},
		}
		if t.prober == nil {
			return out, nil
		}
		media, err := t.prober.Probe(ctx, t.paths.LibraryPath(file.Rel))
		if err != nil {
			t.log.WarnContext(ctx, "ffprobe could not read a library video", "title", titleID, "file", fileID, "error", err)
			return out, nil
		}
		return technicalFileFrom(file.Rel, file.Size, media), nil
	}
	return TechnicalFile{}, ErrLibraryFileNotFound
}

func containsTitle(titles []domain.LibraryTitle, id int64) bool {
	for _, title := range titles {
		if title.ID == id {
			return true
		}
	}
	return false
}

func technicalFileFrom(path string, size int64, media TechnicalMedia) TechnicalFile {
	out := TechnicalFile{
		Path:      path,
		Size:      size,
		Readable:  true,
		Duration:  media.Duration,
		Video:     media.Video,
		Audio:     make([]TechnicalAudio, 0, len(media.Audio)),
		Subtitles: make([]TechnicalSubtitle, 0, len(media.Subtitles)),
	}
	out.Audio = append(out.Audio, media.Audio...)
	out.Subtitles = append(out.Subtitles, media.Subtitles...)
	return out
}
