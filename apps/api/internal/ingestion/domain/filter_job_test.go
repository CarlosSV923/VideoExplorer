package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

func TestAnUploadedFileGoesToFilteringAndIsMarkedOnce(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	job, err := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "Movie.mkv", BatchID: "b-1"}, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.MarkFiltered(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("MarkFiltered while uploading: err = %v, want ErrInvalidTransition", err)
	}
	if err := job.MarkUploaded(now); err != nil {
		t.Fatal(err)
	}
	if err := job.StartFiltering(now); err != nil {
		t.Fatalf("StartFiltering: %v", err)
	}
	if job.Status != domain.StatusFiltering || job.FilterDone {
		t.Fatalf("job = %s, filter done %v; want filtering, not done", job.Status, job.FilterDone)
	}
	if err := job.MarkFiltered(now); err != nil {
		t.Fatalf("MarkFiltered: %v", err)
	}
	if !job.FilterDone || job.Status != domain.StatusFiltering {
		t.Errorf("after MarkFiltered: %s, filter done %v; want filtering and done", job.Status, job.FilterDone)
	}
}

func TestNoVideoIsAKnownReasonWithoutRetry(t *testing.T) {
	t.Parallel()

	if !domain.FailureNoVideo.Valid() {
		t.Fatal("no_video is not a known reason")
	}
	if domain.FailureNoVideo.Retryable() {
		t.Error("no_video admits a retry; RF-07 says it does not")
	}
}

func TestDiscardEndsAFilteredJobOnlyFromFiltering(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	job, err := domain.NewUploadJob("job-2", domain.UploadMeta{FileName: "readme.txt", BatchID: "b-1"}, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Discard(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("Discard while uploading: err = %v, want ErrInvalidTransition", err)
	}
	if err := job.MarkUploaded(now); err != nil {
		t.Fatal(err)
	}
	if err := job.StartFiltering(now); err != nil {
		t.Fatal(err)
	}
	if err := job.Discard(now); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if job.Status != domain.StatusDiscarded || !job.Status.Terminal() {
		t.Errorf("job = %s, want discarded and terminal", job.Status)
	}
	if job.Status.CanTransitionTo(domain.StatusFailed) {
		t.Error("a discarded job can still fail")
	}
}
