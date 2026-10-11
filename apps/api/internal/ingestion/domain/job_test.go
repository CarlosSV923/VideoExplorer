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
	domain.StatusFiltering:     {domain.StatusDone, domain.StatusFailed, domain.StatusCancelled},
	domain.StatusDone:          nil,
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
	meta := domain.UploadMeta{FileName: "a.mkv"}

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
		if err := job.Fail("disk full", later); err != nil {
			t.Fatal(err)
		}
		if job.Status != domain.StatusFailed || job.Error != "disk full" {
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
		if err := job.Fail("late", later); !errors.Is(err, domain.ErrInvalidTransition) {
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
