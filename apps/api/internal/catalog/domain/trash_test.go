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
	tests := []struct {
		name    string
		origin  domain.TrashOrigin
		reason  domain.TrashReason
		dir     string
		file    domain.TrashFile
		wantErr bool
	}{
		{name: "the archive of an upload", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", file: file},
		{name: "an unknown origin", origin: "library", reason: domain.TrashReasonExtracted, dir: "a1b2", file: file, wantErr: true},
		{name: "an unknown reason", origin: domain.TrashOriginUpload, reason: "deleted", dir: "a1b2", file: file, wantErr: true},
		{name: "a folder that is a path", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "../a1b2", file: file, wantErr: true},
		{name: "an empty folder", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "", file: file, wantErr: true},
		{name: "a file name with a separator", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", file: domain.TrashFile{Name: "x/y.zip"}, wantErr: true},
		{name: "a dot file name", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", file: domain.TrashFile{Name: ".."}, wantErr: true},
		{name: "a negative size", origin: domain.TrashOriginUpload, reason: domain.TrashReasonExtracted, dir: "a1b2", file: domain.TrashFile{Name: "x.zip", Size: -1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NewTrashEntry(tt.origin, tt.reason, tt.dir, tt.file, now)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidTrash) {
					t.Fatalf("err = %v, want ErrInvalidTrash", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got.Dir != tt.dir || !got.TrashedAt.Equal(now) || len(got.Files) != 1 || got.Files[0] != tt.file {
				t.Errorf("entry = %+v", got)
			}
		})
	}
}
