package application_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// rarBytes starts like a RAR 5 archive but is not one, so 7-Zip refuses it: a
// plain .rar with no old volumes beside it ends as archive_corrupt.
var rarBytes = []byte("Rar!\x1A\x07\x01\x00 not a complete archive")

// oldVolumeBytes is the content of an old-style RAR volume (RF-03).
var oldVolumeBytes = []byte("Rar!\x1A\x07\x00 old volume")

// uploadTo admits and completes a file of the given batch, as the tus endpoint does.
func (r *rig) uploadTo(t *testing.T, batch domain.BatchID, name string, data []byte) domain.JobID {
	t.Helper()

	id, err := r.svc.Admit(t.Context(), application.Admission{
		Meta: domain.UploadMeta{FileName: name, BatchID: string(batch)},
		Size: int64(len(data)),
	})
	if err != nil {
		t.Fatalf("Admit %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(r.staging, string(id)), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Completed(t.Context(), id); err != nil {
		t.Fatalf("Completed %s: %v", name, err)
	}
	return id
}

// admitOnly admits a file of the batch without any of its bytes: it is in the
// batch and still arriving.
func (r *rig) admitOnly(t *testing.T, batch domain.BatchID, name string) domain.JobID {
	t.Helper()

	id, err := r.svc.Admit(t.Context(), application.Admission{
		Meta: domain.UploadMeta{FileName: name, BatchID: string(batch)},
		Size: int64(len(oldVolumeBytes)),
	})
	if err != nil {
		t.Fatalf("Admit %s: %v", name, err)
	}
	return id
}

func TestAPlainRARWithItsOldVolumesInTheBatchFailsAsLegacyVolume(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// queued leaves the workers stopped until both files are in the batch, so
		// the volume and the .rar wait together; the single worker then takes the
		// volume first, as it was admitted first.
		queued bool
		// setup brings the old volume and the .rar into the batch and returns the .rar's job.
		setup func(t *testing.T, r *rig) domain.JobID
	}{
		{
			name: "the volume is admitted but still arriving when the .rar completes",
			setup: func(t *testing.T, r *rig) domain.JobID {
				r.admitOnly(t, r.batch, "Movie.r00")
				return r.uploadTo(t, r.batch, "Movie.rar", rarBytes)
			},
		},
		{
			name: "the volume was refused before the .rar arrived",
			setup: func(t *testing.T, r *rig) domain.JobID {
				v := r.uploadTo(t, r.batch, "movie.R01", oldVolumeBytes)
				r.waitFor(t, v, "volume refused", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
				return r.uploadTo(t, r.batch, "Movie.rar", rarBytes)
			},
		},
		{
			name:   "the volume and the .rar wait together and the volume is refused first",
			queued: true,
			setup: func(t *testing.T, r *rig) domain.JobID {
				r.uploadTo(t, r.batch, "Movie.r00", oldVolumeBytes)
				rar := r.uploadTo(t, r.batch, "Movie.rar", rarBytes)
				go r.svc.RunExtraction(t.Context(), 1)
				return rar
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, !tt.queued, nil)
			rar := tt.setup(t, r)
			job := r.waitFor(t, rar, "the .rar refused", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
			if job.Error != domain.FailureLegacyVolume {
				t.Fatalf("the .rar failed with %q, want legacy_volume", job.Error)
			}
			if r.staged(rar) {
				t.Error("the .rar was kept after legacy_volume")
			}
			if _, err := r.svc.Retry(t.Context(), rar); err == nil {
				t.Error("the .rar of an old set can be retried; RF-03 says it cannot")
			}
		})
	}
}

func TestAPlainRARIsNotRefusedForAnotherSetOrAnotherBatch(t *testing.T) {
	t.Parallel()
	need7zz(t)

	tests := []struct {
		name string
		// volume is the old volume admitted beside the .rar; empty for none.
		volume string
		// otherBatch admits the volume in a batch of its own.
		otherBatch bool
	}{
		{name: "no old volume in the batch"},
		{name: "an old volume of another name", volume: "Other.r00"},
		{name: "an old volume of the same name in another batch", volume: "Movie.r00", otherBatch: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			if tt.volume != "" {
				batch := r.batch
				if tt.otherBatch {
					other, err := r.svc.CreateBatch(t.Context(), "", domain.RelationNone)
					if err != nil {
						t.Fatal(err)
					}
					batch = other.ID
				}
				r.admitOnly(t, batch, tt.volume)
			}
			rar := r.uploadTo(t, r.batch, "Movie.rar", rarBytes)
			job := r.waitFor(t, rar, "the .rar refused as corrupt", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
			if job.Error != domain.FailureArchiveCorrupt {
				t.Fatalf("the .rar failed with %q, want archive_corrupt", job.Error)
			}
		})
	}
}

// filterRules are the defaults of VIDEO_EXTENSIONS and SUBTITLE_EXTENSIONS (spec §7),
// which the rigs of these tests filter with.
func filterRules() domain.FilterRules {
	return domain.NewFilterRules(
		[]string{".mkv", ".mp4", ".avi", ".m4v", ".ts", ".m2ts", ".mov", ".wmv", ".webm", ".mpg", ".mpeg"},
		[]string{".srt", ".ass", ".ssa", ".sub", ".idx", ".vtt"},
	)
}
