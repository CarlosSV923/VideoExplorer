package application_test

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

// mb is a megabyte of the spec's sample limit (decimal, see domain.SampleMaxSize).
const mb = 1000 * 1000

// meta is the upload metadata of a file dropped in the rig's batch: its path in
// the folder dropped in the browser when relPath has folders (RF-01a).
func (r *rig) meta(relPath string) domain.UploadMeta {
	meta := domain.UploadMeta{FileName: path.Base(relPath), BatchID: string(r.batch)}
	if strings.Contains(relPath, "/") {
		meta.RelativePath = relPath
	}
	return meta
}

// drop admits a file dropped in the rig's batch and completes it (RF-09a).
func (r *rig) drop(t *testing.T, relPath string, data []byte) domain.JobID {
	t.Helper()

	return r.admitComplete(t, r.meta(relPath), data, int64(len(data)))
}

// dropSparse is drop for a file of size bytes that holds no data: its bytes are zeros
// that were never written, so a file of 150 MB costs nothing on disk.
func (r *rig) dropSparse(t *testing.T, relPath string, size int64) domain.JobID {
	t.Helper()

	return r.admitComplete(t, r.meta(relPath), nil, size)
}

// admitComplete admits a file, stages its data (zeros past it, when there are fewer
// bytes than size) and completes it, as the tus endpoint does.
func (r *rig) admitComplete(t *testing.T, meta domain.UploadMeta, data []byte, size int64) domain.JobID {
	t.Helper()

	id, err := r.svc.Admit(t.Context(), application.Admission{Meta: meta, Size: size})
	if err != nil {
		t.Fatalf("Admit %s: %v", meta.FileName, err)
	}
	staged := filepath.Join(r.staging, string(id))
	if err := os.WriteFile(staged, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(staged, size); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Completed(t.Context(), id); err != nil {
		t.Fatalf("Completed %s: %v", meta.FileName, err)
	}
	return id
}

// trashedNamed returns the trash entries of the files called name.
func (r *rig) trashedNamed(name string) []application.TrashPut {
	var out []application.TrashPut
	for _, p := range r.trash.puts() {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// reachedFilter waits until the job has been through the filter or has failed.
func (r *rig) reachedFilter(t *testing.T, id domain.JobID) domain.UploadJob {
	t.Helper()

	return r.waitFor(t, id, "filtered or failed", func(j domain.UploadJob) bool {
		return j.FilterDone || j.Status == domain.StatusFailed
	})
}

// service builds a service over the rig's database and folders, as the process
// started after a restart does, with the given filter records and trash.
func (r *rig) service(entries domain.FilterRepository, trash application.Trash) *application.Service {
	space := func(string) (uint64, error) { return 1 << 40, nil }
	return application.NewService(ingestionsqlite.NewBatchRepository(r.conn), ingestionsqlite.NewJobRepository(r.conn), r.pub, nil, time.Now).
		WithStaging(diskStaging{dir: r.staging}).
		WithExtraction(workspace.New(r.staging, space), realExtractor(), trash).
		WithFiltering(entries, filterRules())
}

func TestALooseFileIsFilteredAsItWasUploaded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		size int64
		// wantKept: the file stays in staging and its job is filtered; otherwise the job fails with no_video.
		wantKept bool
	}{
		{name: "a video in upper case", file: "Movie.MKV", size: 200 * mb, wantKept: true},
		{name: "a subtitle on its own is kept", file: "Movie.es.srt", size: 20_000, wantKept: true},
		{name: "a text file is discarded", file: "notes.txt", size: 200, wantKept: false},
		{name: "a small sample is discarded", file: "sample.mkv", size: 40 * mb, wantKept: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			id := r.dropSparse(t, tt.file, tt.size)
			job := r.reachedFilter(t, id)

			if tt.wantKept {
				if job.Status != domain.StatusFiltering || !r.staged(id) || len(r.trash.puts()) != 0 {
					t.Fatalf("job = %s, staged %v, trashed %d; want filtering, kept in staging, nothing trashed", job.Status, r.staged(id), len(r.trash.puts()))
				}
				kept, err := r.svc.Kept(t.Context(), id)
				if err != nil || len(kept) != 1 || kept[0].Path != tt.file {
					t.Fatalf("Kept = %+v, %v; want the file itself", kept, err)
				}
				return
			}
			if job.Status != domain.StatusFailed || job.Error != domain.FailureNoVideo {
				t.Fatalf("job = %s (%q), want failed with no_video", job.Status, job.Error)
			}
			if r.staged(id) {
				t.Error("the discarded file is still in staging")
			}
			puts := r.trash.puts()
			if len(puts) != 1 || puts[0].Name != tt.file || puts[0].Reason != application.TrashReasonFiltered {
				t.Fatalf("trash = %+v, want the file with the filtered reason", puts)
			}
			if _, err := r.svc.Retry(t.Context(), id); !errors.Is(err, application.ErrCannotRetry) {
				t.Errorf("Retry: err = %v, want ErrCannotRetry (no_video does not admit a retry)", err)
			}
		})
	}
}

func TestAnArchiveKeepsItsVideoAndTrashesEverythingElse(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	id := r.upload(t, "Movie.zip", zipBytes(t,
		zipEntry{name: "Movie/Movie.mkv", content: "fictitious video bytes", mode: 0o644},
		zipEntry{name: "Movie/sample.mkv", content: "small sample", mode: 0o644},
		zipEntry{name: "Movie/Movie.nfo", content: "metadata", mode: 0o644},
		zipEntry{name: "Movie/poster.jpg", content: "image", mode: 0o644},
		zipEntry{name: "Samples/clip.mkv", content: "clip", mode: 0o644},
		zipEntry{name: "Movie/Movie.es.srt", content: "1\n00:00:01,000 --> 00:00:02,000\nHola\n", mode: 0o644},
	))
	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFiltering {
		t.Fatalf("job = %s (%q), want filtering", job.Status, job.Error)
	}

	for _, kept := range []string{"Movie/Movie.mkv", "Movie/Movie.es.srt"} {
		if _, err := os.Stat(filepath.Join(r.extractionDir(id), filepath.FromSlash(kept))); err != nil {
			t.Errorf("%s was not kept in staging: %v", kept, err)
		}
	}
	for _, gone := range []string{"Movie/sample.mkv", "Movie/Movie.nfo", "Movie/poster.jpg", "Samples/clip.mkv"} {
		if _, err := os.Stat(filepath.Join(r.extractionDir(id), filepath.FromSlash(gone))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still in staging: %v", gone, err)
		}
	}

	if n := len(r.trash.puts()); n != 5 {
		t.Fatalf("trash entries = %d, want the archive and its four discarded files: %+v", n, r.trash.puts())
	}
	discarded := map[string]string{
		"sample.mkv": "Movie.zip/Movie/sample.mkv",
		"Movie.nfo":  "Movie.zip/Movie/Movie.nfo",
		"poster.jpg": "Movie.zip/Movie/poster.jpg",
		"clip.mkv":   "Movie.zip/Samples/clip.mkv",
	}
	for name, origin := range discarded {
		got := r.trashedNamed(name)
		if len(got) != 1 {
			t.Fatalf("%s: %d trash entries, want 1", name, len(got))
		}
		if got[0].Reason != application.TrashReasonFiltered || got[0].BatchID != string(r.batch) || got[0].RelativePath != origin {
			t.Errorf("%s: trash entry %+v, want the filtered reason, the batch and the path %q", name, got[0], origin)
		}
	}
	if original := r.trashedNamed("Movie.zip"); len(original) != 1 || original[0].Reason != application.TrashReasonExtracted {
		t.Errorf("the original archive: %+v, want one extracted entry", original)
	}

	kept, err := r.svc.Kept(t.Context(), id)
	if err != nil {
		t.Fatalf("Kept: %v", err)
	}
	var paths []string
	for _, k := range kept {
		paths = append(paths, k.Path)
	}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"Movie/Movie.es.srt", "Movie/Movie.mkv"}) {
		t.Errorf("Kept = %v, want the video and the subtitle", paths)
	}
}

func TestAnArchiveWithNothingToKeepFailsAsNoVideo(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	id := r.upload(t, "Extras.zip", zipBytes(t,
		zipEntry{name: "Extras/Movie.nfo", content: "metadata", mode: 0o644},
		zipEntry{name: "Extras/sample.mkv", content: "small", mode: 0o644},
	))
	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFailed || job.Error != domain.FailureNoVideo {
		t.Fatalf("job = %s (%q), want failed with no_video", job.Status, job.Error)
	}
	if r.staged(id) {
		t.Error("the archive is still in staging after no_video")
	}
	if _, err := os.Stat(r.extractionDir(id)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the extracted folder is still there: %v", err)
	}
	if n := len(r.trash.puts()); n != 3 {
		t.Errorf("trash entries = %d, want the archive and its two files: %+v", n, r.trash.puts())
	}
	if _, err := r.svc.Retry(t.Context(), id); !errors.Is(err, application.ErrCannotRetry) {
		t.Errorf("Retry: err = %v, want ErrCannotRetry", err)
	}
}

func TestADroppedFolderIsFilteredFileByFile(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	video := r.drop(t, "Movie/Movie.mkv", []byte("video"))
	subtitle := r.drop(t, "Movie/Movie.es.srt", []byte("subtitle"))
	readme := r.drop(t, "Movie/readme.txt", []byte("notes"))
	sample := r.dropSparse(t, "Movie/Samples/clip.mkv", 5*mb)

	if job := r.reachedFilter(t, video); job.Status != domain.StatusFiltering {
		t.Errorf("video: %s, want filtering", job.Status)
	}
	if job := r.reachedFilter(t, subtitle); job.Status != domain.StatusFiltering {
		t.Errorf("subtitle: %s, want filtering", job.Status)
	}
	// Each file is a job of its own: a file with nothing to keep fails alone (RF-07).
	if job := r.reachedFilter(t, readme); job.Status != domain.StatusFailed || job.Error != domain.FailureNoVideo {
		t.Errorf("readme: %s (%q), want no_video", job.Status, job.Error)
	}
	if job := r.reachedFilter(t, sample); job.Status != domain.StatusFailed || job.Error != domain.FailureNoVideo {
		t.Errorf("sample in a folder: %s (%q), want no_video", job.Status, job.Error)
	}

	if got := r.trashedNamed("clip.mkv"); len(got) != 1 || got[0].RelativePath != "Movie/Samples/clip.mkv" ||
		got[0].BatchID != string(r.batch) || got[0].Reason != application.TrashReasonFiltered {
		t.Errorf("the sample of the folder: %+v, want its drop path and batch", got)
	}
	if got := r.trashedNamed("readme.txt"); len(got) != 1 || got[0].RelativePath != "Movie/readme.txt" {
		t.Errorf("the readme of the folder: %+v, want its drop path", got)
	}
}

func TestTheSampleLimitIsExactlyTheSpecLimitForLooseFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		size     int64
		wantKept bool
	}{
		{name: "one byte under the limit is a sample", size: domain.SampleMaxSize - 1, wantKept: false},
		{name: "exactly the limit is not a sample", size: domain.SampleMaxSize, wantKept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			id := r.dropSparse(t, "sample.mkv", tt.size)
			job := r.reachedFilter(t, id)
			if kept := job.Status == domain.StatusFiltering; kept != tt.wantKept {
				t.Errorf("a sample.mkv of %d bytes: status %s, want kept %v", tt.size, job.Status, tt.wantKept)
			}
		})
	}
}

// flakyTrash fails the first filtered Put, before anything moves, and tells the test.
type flakyTrash struct {
	inner  application.Trash
	mu     sync.Mutex
	failed bool
	hit    chan struct{}
}

func (f *flakyTrash) Put(ctx context.Context, p application.TrashPut) error {
	f.mu.Lock()
	first := !f.failed && p.Reason == application.TrashReasonFiltered
	if first {
		f.failed = true
	}
	f.mu.Unlock()
	if first {
		f.hit <- struct{}{}
		return errors.New("the trash is busy")
	}
	return f.inner.Put(ctx, p)
}

// flakyMarks fails the first MarkTrashed, after the file moved, and tells the test.
type flakyMarks struct {
	domain.FilterRepository
	mu     sync.Mutex
	failed bool
	hit    chan struct{}
}

func (f *flakyMarks) MarkTrashed(ctx context.Context, unit domain.JobID, p string) error {
	f.mu.Lock()
	first := !f.failed
	f.failed = true
	f.mu.Unlock()
	if first {
		f.hit <- struct{}{}
		return errors.New("the database is busy")
	}
	return f.FilterRepository.MarkTrashed(ctx, unit, p)
}

func TestAFilterInterruptedByARestartDoesNotTrashAFileTwice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// interrupt makes the first step of a discard fail: its move, or the record of it.
		interrupt func(r *rig, hit chan struct{}) (application.Trash, domain.FilterRepository)
	}{
		{
			name: "the move of a discard fails",
			interrupt: func(r *rig, hit chan struct{}) (application.Trash, domain.FilterRepository) {
				return &flakyTrash{inner: r.trash, hit: hit}, ingestionsqlite.NewFilterRepository(r.conn)
			},
		},
		{
			name: "the discard is moved but its record is not updated",
			interrupt: func(r *rig, hit chan struct{}) (application.Trash, domain.FilterRepository) {
				return r.trash, &flakyMarks{FilterRepository: ingestionsqlite.NewFilterRepository(r.conn), hit: hit}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, false, nil)
			hit := make(chan struct{}, 1)
			trash, entries := tt.interrupt(r, hit)
			before := r.service(entries, trash)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			go before.RunExtraction(ctx, 1)
			r.svc = before

			id := r.upload(t, "Movie.zip", zipBytes(t,
				zipEntry{name: "Movie/Movie.mkv", content: "video", mode: 0o644},
				zipEntry{name: "Movie/sample.mkv", content: "small", mode: 0o644},
			))
			select {
			case <-hit:
			case <-time.After(extractionGuard):
				t.Fatal("the interruption never happened")
			}

			// The process restarts: a new service over the same database and folders
			// resumes the filter from what it recorded.
			after := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash)
			resumeCtx, resumeCancel := context.WithCancel(t.Context())
			t.Cleanup(resumeCancel)
			go after.RunExtraction(resumeCtx, 1)
			r.svc = after

			job := r.waitFor(t, id, "resumed and filtered", func(j domain.UploadJob) bool { return j.FilterDone })
			if job.Status != domain.StatusFiltering {
				t.Fatalf("job = %s, want filtering", job.Status)
			}
			if got := r.trashedNamed("sample.mkv"); len(got) != 1 {
				t.Errorf("sample.mkv went to the trash %d times, want once", len(got))
			}
			if got := r.trashedNamed("Movie.zip"); len(got) != 1 {
				t.Errorf("the archive went to the trash %d times, want once", len(got))
			}
			kept, err := r.svc.Kept(t.Context(), id)
			if err != nil || len(kept) != 1 || kept[0].Path != "Movie/Movie.mkv" {
				t.Errorf("Kept after the restart = %+v, %v; want the video", kept, err)
			}
		})
	}
}
