package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

func TestIsLegacyVolumeRecognizesTheOldRARNaming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file string
		want bool
	}{
		{file: "Movie.r00", want: true},
		{file: "movie.R01", want: true},
		{file: "Movie.r99", want: true},
		{file: "Movie.rar", want: false},
		{file: "Movie.part1.rar", want: false},
		{file: "Movie.r0", want: false},
		{file: "Movie.r000", want: false},
		{file: "Movie.r00.mkv", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			if got := domain.IsLegacyVolume(tt.file); got != tt.want {
				t.Errorf("IsLegacyVolume(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestLegacyVolumeFailureIsKnownAndNotRetryable(t *testing.T) {
	t.Parallel()

	if !domain.FailureLegacyVolume.Valid() {
		t.Fatal("legacy_volume is not a known reason")
	}
	if domain.FailureLegacyVolume.Retryable() {
		t.Error("legacy_volume admits a retry; RF-03 says it does not")
	}
}

func TestAVolumeKnowsItsSetAndPosition(t *testing.T) {
	t.Parallel()

	meta, err := domain.NewUploadMeta("Movie.7z.002", "", "lote", "3")
	if err != nil {
		t.Fatal(err)
	}
	j, err := domain.NewUploadJob("job2", meta, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !j.IsVolume() || j.VolumeIndex != 2 || j.PartsTotal != 3 || j.VolumeSet == "" {
		t.Errorf("volume = %+v, want set, index 2 and total 3", j)
	}

	meta, err = domain.NewUploadMeta("Movie.mkv", "", "lote", "")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := domain.NewUploadJob("job3", meta, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plain.IsVolume() || plain.VolumeIndex != 0 || plain.PartsTotal != 0 || plain.VolumeSet != "" {
		t.Errorf("plain file = %+v, want no set", plain)
	}
}

func TestVolumeTransitionsWaitClaimAndResume(t *testing.T) {
	t.Parallel()

	volume := func(t *testing.T) *domain.UploadJob {
		t.Helper()
		meta, err := domain.NewUploadMeta("Movie.7z.001", "", "lote", "2")
		if err != nil {
			t.Fatal(err)
		}
		j, err := domain.NewUploadJob("job1", meta, 10, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := j.MarkUploaded(time.Now()); err != nil {
			t.Fatal(err)
		}
		return j
	}

	t.Run("an uploaded volume waits for its set", func(t *testing.T) {
		t.Parallel()

		j := volume(t)
		if err := j.WaitForParts(time.Now()); err != nil || j.Status != domain.StatusWaitingParts {
			t.Fatalf("WaitForParts: %v, status %s", err, j.Status)
		}
	})

	t.Run("a waiting volume is claimed under the group of its set", func(t *testing.T) {
		t.Parallel()

		j := volume(t)
		if err := j.WaitForParts(time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := j.ClaimSet("job1", time.Now()); err != nil {
			t.Fatalf("ClaimSet: %v", err)
		}
		if j.Status != domain.StatusExtracting || j.VolumeGroup != "job1" {
			t.Errorf("claimed = %s group %q, want extracting under job1", j.Status, j.VolumeGroup)
		}
	})

	t.Run("only a volume of a set can be claimed, and with a group", func(t *testing.T) {
		t.Parallel()

		j := volume(t)
		if err := j.WaitForParts(time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := j.ClaimSet("", time.Now()); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("claim without a group: err = %v", err)
		}
		plain, err := domain.NewUploadJob("job9", mustMeta(t, "Movie.mkv", ""), 10, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := plain.MarkUploaded(time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := plain.ClaimSet("job9", time.Now()); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("claim of a plain file: err = %v", err)
		}
	})

	t.Run("a restart puts an interrupted volume back to waiting, without its group", func(t *testing.T) {
		t.Parallel()

		j := volume(t)
		if err := j.WaitForParts(time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := j.ClaimSet("job1", time.Now()); err != nil {
			t.Fatal(err)
		}
		if !j.ResumeVolumeAfterRestart(time.Now()) {
			t.Fatal("an extracting volume was not resumed")
		}
		if j.Status != domain.StatusWaitingParts || j.VolumeGroup != "" {
			t.Errorf("resumed = %s group %q, want waiting_parts without a group", j.Status, j.VolumeGroup)
		}
	})
}

func mustMeta(t *testing.T, name, parts string) domain.UploadMeta {
	t.Helper()
	meta, err := domain.NewUploadMeta(name, "", "lote", parts)
	if err != nil {
		t.Fatal(err)
	}
	return meta
}
