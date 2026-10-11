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
		{name: "an unknown origin", origin: "library", reason: domain.TrashReasonExtracted, dir: "a1b2", from: from, file: file, wantErr: true},
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
