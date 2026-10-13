package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

func TestFilterAndMoveFailuresRetryWithTheFilter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name            string
		reason          domain.FailureReason
		retryable       bool
		retriesExtract  bool
		wantFilterAfter bool
	}{
		{name: "filter_failed retries with the filter", reason: domain.FailureFilterFailed, retryable: true, wantFilterAfter: true},
		{name: "move_failed retries with the filter", reason: domain.FailureMoveFailed, retryable: true, wantFilterAfter: true},
		{name: "no_space retries the extraction", reason: domain.FailureNoSpace, retryable: true, retriesExtract: true},
		{name: "trash_failed retries the trash move, not the filter", reason: domain.FailureTrashFailed, retryable: true},
		{name: "no_video cannot be retried", reason: domain.FailureNoVideo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.reason.Retryable(); got != tt.retryable {
				t.Errorf("Retryable = %v, want %v", got, tt.retryable)
			}
			if got := tt.reason.RetriesExtraction(); got != tt.retriesExtract {
				t.Errorf("RetriesExtraction = %v, want %v", got, tt.retriesExtract)
			}
			if !tt.reason.Valid() {
				t.Errorf("%s is not a valid reason", tt.reason)
			}

			job := &domain.UploadJob{ID: "j1", FileName: "a.zip", Status: domain.StatusFiltering}
			if err := job.Fail(tt.reason, now); err != nil {
				t.Fatalf("Fail: %v", err)
			}
			err := job.RetryFiltering(now)
			switch {
			case tt.wantFilterAfter && (err != nil || job.Status != domain.StatusFiltering || job.Error != ""):
				t.Errorf("RetryFiltering = %v, status %s, reason %q; want filtering with no reason", err, job.Status, job.Error)
			case !tt.wantFilterAfter && !errors.Is(err, domain.ErrInvalidTransition):
				t.Errorf("RetryFiltering = %v; want ErrInvalidTransition", err)
			}
		})
	}
}

func TestFilteredJobIsDoneOnlyFromFiltering(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	job := &domain.UploadJob{ID: "j1", FileName: "a.mkv", Status: domain.StatusUploaded}
	if err := job.Done(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("Done from uploaded = %v, want ErrInvalidTransition", err)
	}

	job.Status = domain.StatusFiltering
	job.FilterDone = true
	if err := job.Done(now); err != nil {
		t.Fatalf("Done from filtering: %v", err)
	}
	if job.Status != domain.StatusDone {
		t.Errorf("status = %s, want done", job.Status)
	}
}

func TestTrashFailureIsRetriedOnlyByTheTrashMove(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		reason domain.FailureReason
		want   error
	}{
		{name: "trash_failed goes back to filtering", reason: domain.FailureTrashFailed},
		{name: "move_failed cannot be retried by the trash move", reason: domain.FailureMoveFailed, want: domain.ErrInvalidTransition},
		{name: "no_space cannot be retried by the trash move", reason: domain.FailureNoSpace, want: domain.ErrInvalidTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			job := &domain.UploadJob{ID: "j1", FileName: "a.zip", Status: domain.StatusFiltering, FilterDone: true}
			if err := job.Fail(tt.reason, now); err != nil {
				t.Fatalf("Fail: %v", err)
			}
			err := job.RetryTrash(now)
			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Errorf("RetryTrash = %v, want %v", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("RetryTrash: %v", err)
			}
			if job.Status != domain.StatusFiltering || job.Error != "" {
				t.Errorf("status %s, reason %q; want filtering with no reason", job.Status, job.Error)
			}
			if !job.FilterDone {
				t.Error("the filter's mark was cleared: the content was filtered already")
			}
		})
	}
}

func TestRetryExtractionRefusesTheFilterReasons(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)
	job := &domain.UploadJob{ID: "j1", FileName: "a.zip", Status: domain.StatusFiltering}
	if err := job.Fail(domain.FailureMoveFailed, now); err != nil {
		t.Fatal(err)
	}
	if err := job.RetryExtraction(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("RetryExtraction of move_failed = %v, want ErrInvalidTransition", err)
	}
}
