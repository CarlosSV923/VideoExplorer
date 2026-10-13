package domain_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// copied builds a file that Samba copied into an entry, with its magic bytes already read.
func copied(entry int64, rel string, archive bool) domain.SambaFile {
	name := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		name = rel[i+1:]
	}
	return domain.SambaFile{Entry: entry, Rel: rel, Name: name, Size: 100, Source: "/lib/" + rel, Archive: archive}
}

// unitNames lists the units as "entry:rel1,rel2" for a compact comparison.
func unitNames(units []domain.SambaUnit) []string {
	var out []string
	for _, u := range units {
		var rels []string
		for _, f := range u.Files {
			rels = append(rels, f.Rel)
		}
		out = append(out, fmt.Sprintf("%d:%s", u.Entry, strings.Join(rels, ",")))
	}
	return out
}

func TestGroupSambaTakesOnlySettledArchives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []domain.SambaFile
		want  []string
		sets  []bool
	}{
		{
			name:  "a loose archive is one unit",
			files: []domain.SambaFile{copied(1, "", true)},
			want:  []string{"1:"},
		},
		{
			name:  "an archive inside a folder keeps its place",
			files: []domain.SambaFile{copied(1, "Pack/movie.zip", true), copied(1, "Pack/readme.txt", false)},
			want:  []string{"1:Pack/movie.zip"},
		},
		{
			name: "an archive still being copied waits",
			files: []domain.SambaFile{
				{Entry: 1, Rel: "movie.zip", Name: "movie.zip", Size: 100, Copying: true, Archive: true},
			},
		},
		{
			name: "an empty archive is not taken",
			files: []domain.SambaFile{
				{Entry: 1, Rel: "movie.zip", Name: "movie.zip", Size: 0, Archive: true},
			},
		},
		{
			name:  "a file that is not an archive by its bytes is left alone, whatever its name",
			files: []domain.SambaFile{copied(1, "movie.zip", false), copied(1, "notes.txt", false)},
		},
		{
			name:  "the same archive in two entries is two units",
			files: []domain.SambaFile{copied(1, "a.zip", true), copied(2, "a.zip", true)},
			want:  []string{"1:a.zip", "2:a.zip"},
		},
		{
			name: "a complete set in one folder is one unit, in volume order",
			files: []domain.SambaFile{
				copied(1, "Pack/movie.7z.002", false), copied(1, "Pack/movie.7z.001", true), copied(1, "Pack/movie.7z.003", false),
			},
			want: []string{"1:Pack/movie.7z.001,Pack/movie.7z.002,Pack/movie.7z.003"},
			sets: []bool{true},
		},
		{
			name: "a set with a hole is still one unit: the extraction reports the missing part",
			files: []domain.SambaFile{
				copied(1, "movie.part1.rar", true), copied(1, "movie.part2.rar", false), copied(1, "movie.part4.rar", false),
			},
			want: []string{"1:movie.part1.rar,movie.part2.rar,movie.part4.rar"},
			sets: []bool{true},
		},
		{
			name: "a set whose first volume is not an archive is not taken, and none of its volumes alone",
			files: []domain.SambaFile{
				copied(1, "movie.7z.001", false), copied(1, "movie.7z.002", true),
			},
		},
		{
			name: "a set with a volume still being copied waits as a whole",
			files: []domain.SambaFile{
				copied(1, "movie.7z.001", true),
				{Entry: 1, Rel: "movie.7z.002", Name: "movie.7z.002", Size: 100, Copying: true},
			},
		},
		{
			name: "an old RAR is one unit with its old volumes, after it",
			files: []domain.SambaFile{
				copied(1, "movie.r01", false), copied(1, "movie.rar", true), copied(1, "movie.r00", false),
			},
			want: []string{"1:movie.rar,movie.r00,movie.r01"},
		},
		{
			name: "an old RAR with an old volume still being copied waits",
			files: []domain.SambaFile{
				copied(1, "movie.rar", true),
				{Entry: 1, Rel: "movie.r00", Name: "movie.r00", Size: 100, Copying: true},
			},
		},
		{
			name:  "an old volume alone is an unit of its own, as in an upload (RF-03)",
			files: []domain.SambaFile{copied(1, "movie.r00", false)},
			want:  []string{"1:movie.r00"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			units := domain.GroupSamba(tt.files)
			got := unitNames(units)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("units = %v, want %v", got, tt.want)
			}
			for i, u := range units {
				if i < len(tt.sets) && u.Set != tt.sets[i] {
					t.Fatalf("unit %d Set = %v, want %v", i, u.Set, tt.sets[i])
				}
			}
		})
	}
}

func TestSambaBatchAndJobKeepTheirOrigin(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		fileName   string
		partsTotal int
		wantErr    bool
	}{
		{name: "a single archive", fileName: "movie.zip"},
		{name: "a volume with its total", fileName: "movie.7z.001", partsTotal: 3},
		{name: "a volume without its total is refused", fileName: "movie.7z.001", wantErr: true},
		{name: "a total of one volume is refused", fileName: "movie.7z.001", partsTotal: 1, wantErr: true},
		{name: "a name with a separator is refused", fileName: "a/b.zip", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			batch, err := domain.NewSambaBatch("batch-1", "movie", now)
			if err != nil {
				t.Fatalf("NewSambaBatch: %v", err)
			}
			if batch.Origin != domain.OriginSamba || batch.Relation != domain.RelationNone {
				t.Fatalf("batch = origin %q relation %q, want samba and none", batch.Origin, batch.Relation)
			}
			job, err := domain.NewSambaJob("job-1", batch.ID, tt.fileName, 10, tt.partsTotal, now)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NewSambaJob accepted an unusable copy")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewSambaJob: %v", err)
			}
			if !job.IsSamba() || job.Status != domain.StatusUploading || job.BatchID != batch.ID {
				t.Fatalf("job = %+v, want a samba job uploading in its batch", job)
			}
			if job.IsVolume() != (tt.partsTotal > 0) || job.PartsTotal != tt.partsTotal {
				t.Fatalf("job volume = %v parts = %d, want %d parts", job.IsVolume(), job.PartsTotal, tt.partsTotal)
			}
		})
	}
}

func TestOnlyAFailedSambaCopyThatARetryCannotFixIsDismissable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		origin domain.BatchOrigin
		status domain.Status
		reason domain.FailureReason
		want   bool
	}{
		{name: "a failed copy that a retry cannot fix", origin: domain.OriginSamba, status: domain.StatusFailed, reason: domain.FailureLegacyVolume, want: true},
		{name: "a failed copy with a missing part", origin: domain.OriginSamba, status: domain.StatusFailed, reason: domain.FailurePartsMissing, want: true},
		{name: "a failed copy that a retry can fix", origin: domain.OriginSamba, status: domain.StatusFailed, reason: domain.FailureArchiveCorrupt},
		{name: "a copy that is still uploading", origin: domain.OriginSamba, status: domain.StatusUploading},
		{name: "a copy that needs its password", origin: domain.OriginSamba, status: domain.StatusNeedsPassword},
		{name: "a failed upload", origin: domain.OriginUpload, status: domain.StatusFailed, reason: domain.FailureLegacyVolume},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := &domain.UploadJob{ID: "j", FileName: "movie.zip", Origin: tt.origin, Status: tt.status, Error: tt.reason}
			if got := j.Dismissable(); got != tt.want {
				t.Fatalf("Dismissable = %v, want %v", got, tt.want)
			}
			err := j.Dismiss(now)
			if (err == nil) != tt.want {
				t.Fatalf("Dismiss error = %v, want refused=%v", err, !tt.want)
			}
			if err == nil && !j.Dismissed {
				t.Fatal("Dismiss did not mark the job")
			}
		})
	}
}
