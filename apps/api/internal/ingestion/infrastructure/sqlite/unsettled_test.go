package sqlite_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// TestUnsettledReportsOnlyJobsStillOnTheirWay: a batch is held while one of its jobs is still on
// its way (RF-07, RF-08a); a job that ended, or that is filtered and waits for its batch, does not.
func TestUnsettledReportsOnlyJobsStillOnTheirWay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		noJob      bool
		status     domain.Status
		filterDone bool
		// otherBatch puts the job in another batch: it does not hold this one.
		otherBatch bool
		want       bool
	}{
		{name: "a batch without jobs is not held", noJob: true, want: false},
		{name: "an upload in progress holds the batch", status: domain.StatusUploading, want: true},
		{name: "an uploaded file holds the batch", status: domain.StatusUploaded, want: true},
		{name: "a volume waiting for its set holds the batch", status: domain.StatusWaitingParts, want: true},
		{name: "an extraction holds the batch", status: domain.StatusExtracting, want: true},
		{name: "a wait for a password holds the batch", status: domain.StatusNeedsPassword, want: true},
		{name: "a file being filtered holds the batch", status: domain.StatusFiltering, want: true},
		{name: "a filtered file waits for its batch and does not hold it", status: domain.StatusFiltering, filterDone: true, want: false},
		{name: "a done job does not hold the batch", status: domain.StatusDone, want: false},
		{name: "a discarded job does not hold the batch", status: domain.StatusDiscarded, want: false},
		{name: "a failed job does not hold the batch", status: domain.StatusFailed, want: false},
		{name: "a cancelled job does not hold the batch", status: domain.StatusCancelled, want: false},
		{name: "an upload of another batch does not hold it", status: domain.StatusUploading, otherBatch: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := open(t, t.TempDir())
			batches := sqlite.NewBatchRepository(conn)
			jobs := sqlite.NewJobRepository(conn)
			ctx := t.Context()
			for _, id := range []domain.BatchID{"b-held", "b-other"} {
				if err := batches.Create(ctx, &domain.Batch{ID: id, Relation: domain.RelationEpisodes, CreatedAt: created}); err != nil {
					t.Fatal(err)
				}
			}
			if !tt.noJob {
				batch := domain.BatchID("b-held")
				if tt.otherBatch {
					batch = "b-other"
				}
				job, err := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "Show.S01E01.mkv", BatchID: string(batch)}, 10, created)
				if err != nil {
					t.Fatal(err)
				}
				if err := jobs.Create(ctx, job); err != nil {
					t.Fatal(err)
				}
				job.Status = tt.status
				job.FilterDone = tt.filterDone
				if err := jobs.Save(ctx, job); err != nil {
					t.Fatal(err)
				}
			}

			got, err := jobs.Unsettled(ctx, "b-held")
			if err != nil {
				t.Fatalf("Unsettled: %v", err)
			}
			if got != tt.want {
				t.Errorf("Unsettled = %v, want %v", got, tt.want)
			}
		})
	}
}
