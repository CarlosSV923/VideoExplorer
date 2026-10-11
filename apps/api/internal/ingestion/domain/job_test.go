package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// allowed is the transition rule of RF-10a written out again, so the test checks
// the table in job.go against the spec and not against itself.
var allowed = map[domain.Status][]domain.Status{
	domain.StatusUploading:     {domain.StatusUploaded, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusUploaded:      {domain.StatusWaitingParts, domain.StatusExtracting, domain.StatusFiltering, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusWaitingParts:  {domain.StatusExtracting, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusExtracting:    {domain.StatusNeedsPassword, domain.StatusFiltering, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusNeedsPassword: {domain.StatusExtracting, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusFiltering:     {domain.StatusDone, domain.StatusDiscarded, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusDone:          nil,
	domain.StatusDiscarded:     nil,
	domain.StatusFailed:        nil,
	domain.StatusCancelled:     nil,
}

func TestStatusTransitions(t *testing.T) {
	t.Parallel()

	statuses := domain.Statuses()
	if len(statuses) != len(allowed) {
		t.Fatalf("%d states, want the %d of RF-10a", len(statuses), len(allowed))
	}
	for _, from := range statuses {
		want, known := allowed[from]
		if !known {
			t.Fatalf("state %q is not in RF-10a", from)
		}
		for _, to := range statuses {
			t.Run(string(from)+" to "+string(to), func(t *testing.T) {
				t.Parallel()

				expected := false
				for _, w := range want {
					if w == to {
						expected = true
					}
				}
				if got := from.CanTransitionTo(to); got != expected {
					t.Errorf("%s → %s allowed = %v, want %v", from, to, got, expected)
				}
			})
		}
	}
}

func TestTerminalStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status domain.Status
		want   bool
	}{
		{status: domain.StatusUploading, want: false},
		{status: domain.StatusUploaded, want: false},
		{status: domain.StatusFiltering, want: false},
		{status: domain.StatusDone, want: true},
		{status: domain.StatusDiscarded, want: true},
		{status: domain.StatusFailed, want: true},
		{status: domain.StatusCancelled, want: true},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()

			if got := tt.status.Terminal(); got != tt.want {
				t.Errorf("Terminal(%s) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestNewUploadJob(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	meta := domain.UploadMeta{FileName: "E01.mkv", RelativePath: "Show/Season 01/E01.mkv", BatchID: "b1"}
	tests := []struct {
		name    string
		size    int64
		wantErr error
	}{
		{name: "declared size", size: 10},
		{name: "zero bytes", size: 0, wantErr: domain.ErrEmptyFile},
		{name: "negative size", size: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			job, err := domain.NewUploadJob("job-1", meta, tt.size, now)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.size < 0 {
				if err == nil {
					t.Fatal("negative size accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if job.Status != domain.StatusUploading {
				t.Errorf("status = %s, want uploading", job.Status)
			}
			if job.BatchID != "b1" || job.FileName != "E01.mkv" || job.RelativePath != meta.RelativePath || job.Size != 10 {
				t.Errorf("job = %+v, does not keep the declared values", *job)
			}
			if !job.CreatedAt.Equal(now) || !job.UpdatedAt.Equal(now) {
				t.Errorf("timestamps = %v / %v, want %v", job.CreatedAt, job.UpdatedAt, now)
			}
		})
	}
}

func TestUploadJobUploadTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	meta := domain.UploadMeta{FileName: "a.mkv", BatchID: "b1"}

	t.Run("uploading to uploaded keeps the time of the change", func(t *testing.T) {
		t.Parallel()

		job, _ := domain.NewUploadJob("job-1", meta, 10, now)
		if err := job.MarkUploaded(later); err != nil {
			t.Fatal(err)
		}
		if job.Status != domain.StatusUploaded || !job.UpdatedAt.Equal(later) || !job.CreatedAt.Equal(now) {
			t.Errorf("job = %s updated %v created %v", job.Status, job.UpdatedAt, job.CreatedAt)
		}
	})

	t.Run("uploading to failed keeps the reason", func(t *testing.T) {
		t.Parallel()

		job, _ := domain.NewUploadJob("job-1", meta, 10, now)
		if err := job.Fail(domain.FailureUploadMissing, later); err != nil {
			t.Fatal(err)
		}
		if job.Status != domain.StatusFailed || job.Error != domain.FailureUploadMissing {
			t.Errorf("job = %s error %q, want failed with the reason", job.Status, job.Error)
		}
	})

	t.Run("uploading to cancelled", func(t *testing.T) {
		t.Parallel()

		job, _ := domain.NewUploadJob("job-1", meta, 10, now)
		if err := job.Cancel(later); err != nil {
			t.Fatal(err)
		}
		if job.Status != domain.StatusCancelled {
			t.Errorf("status = %s, want cancelled", job.Status)
		}
	})

	t.Run("refused moves leave the job unchanged", func(t *testing.T) {
		t.Parallel()

		job, _ := domain.NewUploadJob("job-1", meta, 10, now)
		if err := job.MarkUploaded(later); err != nil {
			t.Fatal(err)
		}
		if err := job.MarkUploaded(later); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("uploaded to uploaded: err = %v, want ErrInvalidTransition", err)
		}
		if err := job.Cancel(later); err != nil {
			t.Fatalf("uploaded to cancelled: %v", err)
		}
		if err := job.Fail(domain.FailureUploadAbandoned, later); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("cancelled to failed: err = %v, want ErrInvalidTransition", err)
		}
		if job.Error != "" {
			t.Errorf("error = %q after a refused Fail, want none", job.Error)
		}
		if err := job.Cancel(later); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("cancelled to cancelled: err = %v, want ErrInvalidTransition", err)
		}
		if job.Status != domain.StatusCancelled {
			t.Errorf("status = %s, want cancelled after the refused moves", job.Status)
		}
	})
}

func TestNewUploadJobNeedsABatch(t *testing.T) {
	t.Parallel()

	// Every upload belongs to a batch (RF-09a): a job without one is refused.
	_, err := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "a.mkv"}, 10, time.Now())
	if !errors.Is(err, domain.ErrInvalidMetadata) {
		t.Fatalf("err = %v, want ErrInvalidMetadata", err)
	}
}

func TestRecordProgressOnlyMovesForwardWhileUploading(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	meta := domain.UploadMeta{FileName: "a.mkv", BatchID: "lote-1"}
	tests := []struct {
		name     string
		received []int64
		wantLast int64
		wantSet  []bool
	}{
		{name: "grows", received: []int64{4, 7}, wantLast: 7, wantSet: []bool{true, true}},
		{name: "never goes back", received: []int64{6, 3}, wantLast: 6, wantSet: []bool{true, false}},
		{name: "same count is no change", received: []int64{5, 5}, wantLast: 5, wantSet: []bool{true, false}},
		{name: "capped at the declared size", received: []int64{99}, wantLast: 10, wantSet: []bool{true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			job, _ := domain.NewUploadJob("job-1", meta, 10, now)
			for i, r := range tt.received {
				if got := job.RecordProgress(r, now.Add(time.Second)); got != tt.wantSet[i] {
					t.Errorf("RecordProgress(%d) changed = %v, want %v", r, got, tt.wantSet[i])
				}
			}
			if job.Received != tt.wantLast {
				t.Errorf("received = %d, want %d", job.Received, tt.wantLast)
			}
		})
	}
}

func TestRecordProgressIsRefusedOnceTheUploadEnded(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	job, _ := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "a.mkv", BatchID: "lote-1"}, 10, now)
	if err := job.MarkUploaded(now); err != nil {
		t.Fatal(err)
	}
	if job.Received != 10 {
		t.Errorf("received = %d after MarkUploaded, want the whole size", job.Received)
	}
	if job.RecordProgress(3, now.Add(time.Second)) {
		t.Error("progress of an uploaded job was recorded")
	}
	if job.Received != 10 {
		t.Errorf("received = %d, want 10", job.Received)
	}
}

func TestFailNeedsAKnownReason(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	job, _ := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "a.mkv", BatchID: "lote-1"}, 10, now)

	if err := job.Fail("something else", now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("unknown reason: err = %v, want ErrInvalidTransition", err)
	}
	if job.Status != domain.StatusUploading {
		t.Errorf("status = %s after a refused reason, want uploading", job.Status)
	}
	if err := job.Fail(domain.FailureUploadAbandoned, now); err != nil {
		t.Fatal(err)
	}
	if job.Error != domain.FailureUploadAbandoned {
		t.Errorf("error = %q, want upload_abandoned", job.Error)
	}
}

func TestReconcileReceivedFollowsTheFileWithoutMovingTheIdleTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	later := now.Add(25 * time.Hour)
	job, _ := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "a.mkv", BatchID: "lote-1"}, 10, now)

	if !job.ReconcileReceived(4) {
		t.Fatal("a file with 4 bytes over a count of 0 was not taken")
	}
	if job.Received != 4 || !job.UpdatedAt.Equal(now) {
		t.Errorf("received %d updated %v, want 4 bytes and the idle time kept at %v", job.Received, job.UpdatedAt, now)
	}
	if job.ReconcileReceived(3) {
		t.Error("a smaller count moved the job back")
	}
	if !job.ReconcileReceived(99) || job.Received != 10 {
		t.Errorf("received = %d, want the declared size at most", job.Received)
	}
	if err := job.Cancel(later); err != nil {
		t.Fatal(err)
	}
	if job.ReconcileReceived(10) {
		t.Error("a job that ended was reconciled")
	}
}
