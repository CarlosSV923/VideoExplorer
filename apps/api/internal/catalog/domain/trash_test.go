package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestNewTrashEntry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	file := domain.TrashFile{Name: "Movie.zip", Size: 1024}
	from := domain.Provenance{BatchID: "b1", RelativePath: "Folder/Movie.zip"}
	tests := []struct {
		name    string
		origin  domain.TrashOrigin
		reason  domain.TrashReason
		dir     string
		from    domain.Provenance
		file    domain.TrashFile
		wantErr bool
	}{
		{name: "the archive of an upload", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: from, file: file},
		{name: "a file the filter discarded", origin: domain.TrashOriginUpload, reason: domain.TrashReasonFiltered, dir: "a1b2", from: from, file: file},
		{name: "a file the user sent from No asignados", origin: domain.TrashOriginUpload, reason: domain.TrashReasonRemoved, dir: "a1b2", from: from, file: file},
		{name: "a file copied by Samba", origin: domain.TrashOriginSamba, reason: domain.TrashReasonRemoved, dir: "a1b2", from: domain.Provenance{RelativePath: "Show/S01/x.mkv"}, file: file},
		{name: "a Samba file with a batch", origin: domain.TrashOriginSamba, reason: domain.TrashReasonRemoved, dir: "a1b2", from: domain.Provenance{BatchID: "b1", RelativePath: "x.mkv"}, file: file, wantErr: true},
		{name: "an unknown origin", origin: "book", reason: domain.TrashReasonExtracted, dir: "a1b2", from: from, file: file, wantErr: true},
		{name: "an unknown reason", origin: domain.TrashOriginUpload, reason: "deleted", dir: "a1b2", from: from, file: file, wantErr: true},
		{name: "a folder that is a path", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "../a1b2", from: from, file: file, wantErr: true},
		{name: "an empty folder", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "", from: from, file: file, wantErr: true},
		{name: "a file name with a separator", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: from, file: domain.TrashFile{Name: "x/y.zip"}, wantErr: true},
		{name: "a dot file name", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: from, file: domain.TrashFile{Name: ".."}, wantErr: true},
		{name: "a negative size", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: from, file: domain.TrashFile{Name: "x.zip", Size: -1}, wantErr: true},
		{name: "no batch", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: domain.Provenance{RelativePath: "Movie.zip"}, file: file, wantErr: true},
		{name: "a batch that is a path", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: domain.Provenance{BatchID: "../b1", RelativePath: "Movie.zip"}, file: file, wantErr: true},
		{name: "no relative path", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: domain.Provenance{BatchID: "b1"}, file: file, wantErr: true},
		{name: "an absolute relative path", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", from: domain.Provenance{BatchID: "b1", RelativePath: "/etc/Movie.zip"}, file: file, wantErr: true},
		{name: "a relative path with a dot dot", origin: domain.TrashOriginUpload, reason: domain.TrashReasonFiltered, dir: "a1b2", from: domain.Provenance{BatchID: "b1", RelativePath: "Movie/../../x.mkv"}, file: file, wantErr: true},
		{name: "a relative path with an empty part", origin: domain.TrashOriginUpload, reason: domain.TrashReasonFiltered, dir: "a1b2", from: domain.Provenance{BatchID: "b1", RelativePath: "Movie//x.mkv"}, file: file, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NewTrashEntry(tt.origin, tt.reason, tt.dir, tt.from, tt.file, now)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidTrash) {
					t.Fatalf("err = %v, want ErrInvalidTrash", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got.Dir != tt.dir || !got.TrashedAt.Equal(now) || len(got.Files) != 1 || got.Files[0] != tt.file ||
				got.BatchID != tt.from.BatchID || got.RelativePath != tt.from.RelativePath {
				t.Errorf("entry = %+v", got)
			}
		})
	}
}

func TestNewTrashEntryOfAReplacedLibraryFile(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	video := domain.TrashFile{Name: "Iron Man (2008).mkv", Size: 9}
	subtitle := domain.TrashFile{Name: "Iron Man (2008).es-419.srt", Size: 1}
	libraryFrom := domain.Provenance{RelativePath: "Movies/Iron Man (2008)/Iron Man (2008).mkv"}
	tests := []struct {
		name    string
		origin  domain.TrashOrigin
		reason  domain.TrashReason
		from    domain.Provenance
		files   []domain.TrashFile
		wantErr bool
	}{
		{name: "a replaced video with its subtitle", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: libraryFrom, files: []domain.TrashFile{video, subtitle}},
		{name: "a replaced file with no subtitle", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: libraryFrom, files: []domain.TrashFile{video}},
		{name: "a library file that was not replaced", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonFiltered, from: libraryFrom, files: []domain.TrashFile{video}, wantErr: true},
		{name: "a library file removed from No asignados", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonRemoved, from: libraryFrom, files: []domain.TrashFile{video}, wantErr: true},
		{name: "a replaced upload is not a library file", origin: domain.TrashOriginUpload, reason: domain.TrashReasonReplaced, from: domain.Provenance{BatchID: "b1", RelativePath: "M/a.mkv"}, files: []domain.TrashFile{video}, wantErr: true},
		{name: "a library file with a batch", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: domain.Provenance{BatchID: "b1", RelativePath: "M/a.mkv"}, files: []domain.TrashFile{video}, wantErr: true},
		{name: "a library file with no relative path", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: domain.Provenance{}, files: []domain.TrashFile{video}, wantErr: true},
		{name: "no file at all", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: libraryFrom, files: nil, wantErr: true},
		{
			name: "two files with the same name in another letter case", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: libraryFrom,
			files: []domain.TrashFile{video, {Name: "IRON MAN (2008).MKV", Size: 2}}, wantErr: true,
		},
		{
			name: "a file with a negative size among the others", origin: domain.TrashOriginLibrary, reason: domain.TrashReasonReplaced, from: libraryFrom,
			files: []domain.TrashFile{video, {Name: "x.srt", Size: -1}}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NewTrashEntryOf(tt.origin, tt.reason, "e1", tt.from, tt.files, now)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidTrash) {
					t.Fatalf("err = %v, want ErrInvalidTrash", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(got.Files) != len(tt.files) || got.Origin != tt.origin || got.Reason != tt.reason || got.BatchID != "" {
				t.Errorf("entry = %+v; want the files and the library origin with no batch", got)
			}
		})
	}
}
