package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

type technicalRepo struct {
	titles []domain.LibraryTitle
	files  map[int64][]domain.LibraryFile
}

func (r *technicalRepo) TitleByFolder(context.Context, string) (domain.LibraryTitle, bool, error) {
	return domain.LibraryTitle{}, false, nil
}

func (r *technicalRepo) Titles(context.Context) ([]domain.LibraryTitle, error) { return r.titles, nil }

func (r *technicalRepo) Files(_ context.Context, id int64) ([]domain.LibraryFile, error) {
	return r.files[id], nil
}

type technicalPaths struct{}

func (technicalPaths) LibraryPath(elem ...string) string {
	return "/library/" + strings.Join(elem, "/")
}

type technicalProbe struct {
	media  TechnicalMedia
	err    error
	called string
}

func (p *technicalProbe) Probe(_ context.Context, path string) (TechnicalMedia, error) {
	p.called = path
	return p.media, p.err
}

func TestLibraryTechnicalFile(t *testing.T) {
	t.Parallel()

	video := domain.LibraryFile{ID: 11, TitleID: 1, Kind: domain.FileVideo, Rel: "Movies/Iron Man (2008)/Iron Man.mkv", Size: 1234}
	trashed := video
	trashed.ID = 12
	trashed.TrashEntryID = 99
	other := domain.LibraryFile{ID: 22, TitleID: 2, Kind: domain.FileVideo, Rel: "Movies/Other (2000)/Other.mkv", Size: 88}
	titles := []domain.LibraryTitle{
		{ID: 1, Kind: domain.KindMovie, Title: "Iron Man", DiscTitle: "Iron Man", Folder: "Movies/Iron Man (2008)"},
		{ID: 2, Kind: domain.KindMovie, Title: "Other", DiscTitle: "Other", Folder: "Movies/Other (2000)"},
	}
	media := TechnicalMedia{
		Duration:  2*time.Minute + 3*time.Second,
		Video:     TechnicalVideo{Width: 1920, Height: 1080, HDR: true, Codec: "hevc"},
		Audio:     []TechnicalAudio{{Codec: "aac", Channels: 6, Language: "es-419", Title: "Latino", Default: true}},
		Subtitles: []TechnicalSubtitle{{Codec: "subrip", Language: "es-419", Title: "Español", Forced: true, SDH: false, Default: false}},
	}
	tests := []struct {
		name       string
		titleID    int64
		fileID     int64
		files      map[int64][]domain.LibraryFile
		probeError error
		wantErr    error
		wantRead   bool
	}{
		{name: "reads technical information", titleID: 1, fileID: 11, files: map[int64][]domain.LibraryFile{1: {video}}, wantRead: true},
		{name: "probe failure keeps known data", titleID: 1, fileID: 11, files: map[int64][]domain.LibraryFile{1: {video}}, probeError: errors.New("ffprobe failed")},
		{name: "missing file", titleID: 1, fileID: 99, files: map[int64][]domain.LibraryFile{1: {video}}, wantErr: ErrLibraryFileNotFound},
		{name: "file in trash", titleID: 1, fileID: 12, files: map[int64][]domain.LibraryFile{1: {video, trashed}}, wantErr: ErrLibraryFileNotFound},
		{name: "file of another title", titleID: 1, fileID: 22, files: map[int64][]domain.LibraryFile{1: {video}, 2: {other}}, wantErr: ErrLibraryFileNotFound},
		{name: "title missing", titleID: 99, fileID: 11, files: map[int64][]domain.LibraryFile{1: {video}}, wantErr: ErrTitleNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			probe := &technicalProbe{media: media, err: tt.probeError}
			service := NewLibraryTechnical(&technicalRepo{titles: titles, files: tt.files}, technicalPaths{}, probe, nil)
			got, err := service.File(t.Context(), tt.titleID, tt.fileID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("File error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if probe.called != "" {
					t.Errorf("probe called for invalid file: %q", probe.called)
				}
				return
			}
			if got.Readable != tt.wantRead || got.Path != video.Rel || got.Size != video.Size {
				t.Fatalf("known file data = %+v, want readable=%t path=%q size=%d", got, tt.wantRead, video.Rel, video.Size)
			}
			if tt.probeError != nil {
				if got.Duration != 0 || len(got.Audio) != 0 || len(got.Subtitles) != 0 || got.Video != (TechnicalVideo{}) {
					t.Errorf("failed probe returned technical data: %+v", got)
				}
				return
			}
			if got.Duration != media.Duration || got.Video != media.Video || len(got.Audio) != 1 || len(got.Subtitles) != 1 {
				t.Errorf("technical data = %+v, want %+v", got, media)
			}
			if probe.called != "/library/"+video.Rel {
				t.Errorf("probe path = %q, want server-built path", probe.called)
			}
		})
	}
}
