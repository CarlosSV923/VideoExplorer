package application_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// BenchmarkSettleLargeBatch measures the filter and the settlement of one batch of n video files
// (one byte each, named like episodes: no real video). The time covers the admission of the files
// and the moment the last one is in No asignados; reads/op counts the reads of the job and filter
// repositories over it. Not part of task check: run it on its own, with
//
//	go test -run '^$' -bench SettleLargeBatch -benchtime 1x ./internal/ingestion/application/
func BenchmarkSettleLargeBatch(b *testing.B) {
	for _, n := range []int{250, 1000, 3000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			for range b.N {
				b.StopTimer()
				lib := &fakeLibrary{root: filepath.Join(b.TempDir(), "library", "_unassigned"), placed: map[string]map[string]string{}}
				r := newSettleRig(b, lib)
				batch, err := r.svc.CreateBatch(context.Background(), "Show", domain.RelationEpisodes)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				for i := range n {
					name := fmt.Sprintf("Show.S01E%04d.mkv", i+1)
					id, err := r.svc.Admit(context.Background(), application.Admission{
						Meta: domain.UploadMeta{FileName: name, BatchID: string(batch.ID)},
						Size: 1,
					})
					if err != nil {
						b.Fatalf("Admit: %v", err)
					}
					if err := os.WriteFile(filepath.Join(r.staging, string(id)), []byte("v"), 0o600); err != nil {
						b.Fatal(err)
					}
					if err := r.svc.Completed(context.Background(), id); err != nil {
						b.Fatalf("Completed: %v", err)
					}
				}
				waitMoved(b, lib, n)

				b.StopTimer()
				b.ReportMetric(float64(r.jobs.readCount()), "reads/op")
				b.StartTimer()
			}
		})
	}
}

// waitMoved waits until the library holds n files of the batch, or fails after the guard.
func waitMoved(b *testing.B, lib *fakeLibrary, n int) {
	b.Helper()

	deadline := time.Now().Add(10 * time.Minute)
	for lib.renames() < n {
		if time.Now().After(deadline) {
			b.Fatalf("%d of %d files moved", lib.renames(), n)
		}
		time.Sleep(time.Millisecond)
	}
}
