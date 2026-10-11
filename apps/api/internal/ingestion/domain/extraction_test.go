package domain_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// jobIn returns an upload job in the given state, built through the transitions
// the way the service reaches it.
func jobIn(t *testing.T, status domain.Status) *domain.UploadJob {
	t.Helper()

	meta, err := domain.NewUploadMeta("Movie.zip", "", "lote", "")
	if err != nil {
		t.Fatal(err)
	}
	j, err := domain.NewUploadJob("job", meta, 100, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	steps := map[domain.Status][]func() error{
		domain.StatusUploading: {},
		domain.StatusUploaded:  {func() error { return j.MarkUploaded(now) }},
		domain.StatusExtracting: {
			func() error { return j.MarkUploaded(now) },
			func() error { return j.StartExtraction(now) },
		},
		domain.StatusNeedsPassword: {
			func() error { return j.MarkUploaded(now) },
			func() error { return j.StartExtraction(now) },
			func() error { return j.AskPassword(now) },
		},
		domain.StatusFiltering: {
			func() error { return j.MarkUploaded(now) },
			func() error { return j.StartExtraction(now) },
			func() error { return j.FinishExtraction(now) },
		},
		domain.StatusFailed: {
			func() error { return j.MarkUploaded(now) },
			func() error { return j.StartExtraction(now) },
			func() error { return j.Fail(domain.FailureArchiveCorrupt, now) },
		},
	}
	for _, step := range steps[status] {
		if err := step(); err != nil {
			t.Fatalf("reaching %s: %v", status, err)
		}
	}
	if j.Status != status {
		t.Fatalf("job is %s, want %s", j.Status, status)
	}
	return j
}

func TestStartExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		from    domain.Status
		wantErr bool
	}{
		{name: "an uploaded archive", from: domain.StatusUploaded},
		{name: "a password sent to a job that waits for it", from: domain.StatusNeedsPassword},
		{name: "a job still uploading", from: domain.StatusUploading, wantErr: true},
		{name: "a finished extraction", from: domain.StatusFiltering, wantErr: true},
		{name: "a failed job", from: domain.StatusFailed, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := jobIn(t, tt.from)
			err := j.StartExtraction(time.Now())
			if (err != nil) != tt.wantErr {
				t.Fatalf("StartExtraction from %s: err = %v, want error %v", tt.from, err, tt.wantErr)
			}
			if err == nil && (j.Status != domain.StatusExtracting || j.Progress != 0 || j.Error != "") {
				t.Errorf("after start: status %s, progress %d, error %q", j.Status, j.Progress, j.Error)
			}
		})
	}
}

func TestRecordExtractionProgressOnlyMovesForwardWhileExtracting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  domain.Status
		before  []int // progress reports that came before this one
		pct     int
		changed bool
		want    int
	}{
		{name: "first step", status: domain.StatusExtracting, pct: 10, changed: true, want: 10},
		{name: "same step", status: domain.StatusExtracting, before: []int{10}, pct: 10, want: 10},
		{name: "backwards", status: domain.StatusExtracting, before: []int{10}, pct: 5, want: 10},
		{name: "beyond 100 is capped", status: domain.StatusExtracting, before: []int{50}, pct: 140, changed: true, want: 100},
		{name: "not extracting", status: domain.StatusUploaded, pct: 50, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := jobIn(t, tt.status)
			for _, pct := range tt.before {
				j.RecordExtractionProgress(pct, time.Now())
			}
			if got := j.RecordExtractionProgress(tt.pct, time.Now()); got != tt.changed {
				t.Fatalf("changed = %v, want %v", got, tt.changed)
			}
			if j.Progress != tt.want {
				t.Errorf("progress = %d, want %d", j.Progress, tt.want)
			}
		})
	}
}

func TestPasswordPausesAndResumeOfTheExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		act        func(j *domain.UploadJob) error
		wantStatus domain.Status
		wantError  domain.FailureReason
	}{
		{
			name:       "encrypted archive asks for the password, without a reason",
			act:        func(j *domain.UploadJob) error { return j.AskPassword(time.Now()) },
			wantStatus: domain.StatusNeedsPassword,
		},
		{
			name:       "a wrong password says so (RF-10c)",
			act:        func(j *domain.UploadJob) error { return j.RejectPassword(time.Now()) },
			wantStatus: domain.StatusNeedsPassword,
			wantError:  domain.FailureWrongPassword,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := jobIn(t, domain.StatusExtracting)
			if err := tt.act(j); err != nil {
				t.Fatal(err)
			}
			if j.Status != tt.wantStatus || j.Error != tt.wantError {
				t.Errorf("status %s error %q, want %s %q", j.Status, j.Error, tt.wantStatus, tt.wantError)
			}
		})
	}
}

func TestFinishExtractionEndsAtFiltering(t *testing.T) {
	t.Parallel()

	j := jobIn(t, domain.StatusExtracting)
	if err := j.FinishExtraction(time.Now()); err != nil {
		t.Fatal(err)
	}
	if j.Status != domain.StatusFiltering || j.Progress != 100 {
		t.Fatalf("status %s progress %d, want filtering at 100", j.Status, j.Progress)
	}
	if err := j.FinishExtraction(time.Now()); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second finish: err = %v, want ErrInvalidTransition", err)
	}
}

func TestRetryExtractionOnlyForRetryableFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reason  domain.FailureReason
		status  domain.Status
		wantErr bool
	}{
		{name: "no space", reason: domain.FailureNoSpace},
		{name: "corrupt archive", reason: domain.FailureArchiveCorrupt},
		{name: "crc mismatch", reason: domain.FailureCRCMismatch},
		{name: "unsafe archive cannot be retried", reason: domain.FailureArchiveUnsafe, wantErr: true},
		{name: "missing parts cannot be retried", reason: domain.FailurePartsMissing, wantErr: true},
		{name: "abandoned upload cannot be retried", reason: domain.FailureUploadAbandoned, wantErr: true},
		{name: "a job that is not failed", status: domain.StatusNeedsPassword, reason: domain.FailureWrongPassword, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := jobIn(t, domain.StatusExtracting)
			if tt.status == domain.StatusNeedsPassword {
				j = jobIn(t, domain.StatusNeedsPassword)
			} else if err := j.Fail(tt.reason, time.Now()); err != nil {
				t.Fatal(err)
			}
			before := j.Status
			err := j.RetryExtraction(time.Now())
			if (err != nil) != tt.wantErr {
				t.Fatalf("RetryExtraction: err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidTransition) {
					t.Errorf("refusal = %v, want ErrInvalidTransition", err)
				}
				if j.Status != before {
					t.Errorf("a refused retry changed the job from %s to %s", before, j.Status)
				}
				return
			}
			if j.Status != domain.StatusExtracting || j.Error != "" || j.Progress != 0 {
				t.Errorf("after retry: status %s error %q progress %d", j.Status, j.Error, j.Progress)
			}
		})
	}
}

func TestResumeAfterRestartOnlyMovesAnExtractionBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      domain.Status
		wantChanged bool
		wantStatus  domain.Status
	}{
		{name: "an extraction the restart interrupted starts over", status: domain.StatusExtracting, wantChanged: true, wantStatus: domain.StatusUploaded},
		{name: "a job waiting for its password keeps waiting", status: domain.StatusNeedsPassword, wantStatus: domain.StatusNeedsPassword},
		{name: "a finished job is left alone", status: domain.StatusFiltering, wantStatus: domain.StatusFiltering},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := jobIn(t, tt.status)
			if got := j.ResumeAfterRestart(time.Now()); got != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", got, tt.wantChanged)
			}
			if j.Status != tt.wantStatus {
				t.Errorf("status = %s, want %s", j.Status, tt.wantStatus)
			}
		})
	}
}

func TestFailureReasonsOfRFTenC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reason    domain.FailureReason
		valid     bool
		retryable bool
	}{
		{reason: "no_space", valid: true, retryable: true},
		{reason: "archive_corrupt", valid: true, retryable: true},
		{reason: "crc_mismatch", valid: true, retryable: true},
		{reason: "archive_unsafe", valid: true},
		{reason: "wrong_password", valid: true},
		{reason: "parts_mismatch", valid: true},
		{reason: "parts_missing", valid: true},
		{reason: "upload_abandoned", valid: true},
		{reason: "upload_missing", valid: true},
		{reason: "crashed", valid: false},
		{reason: "", valid: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			t.Parallel()

			if got := tt.reason.Valid(); got != tt.valid {
				t.Errorf("Valid = %v, want %v", got, tt.valid)
			}
			if got := tt.reason.Retryable(); got != tt.retryable {
				t.Errorf("Retryable = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestDetectArchiveByMagicBytesOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header []byte
		want   domain.ArchiveFormat
		ok     bool
	}{
		{name: "zip local header", header: []byte("PK\x03\x04\x14\x00\x00\x00"), want: domain.ArchiveZip, ok: true},
		{name: "empty zip", header: []byte("PK\x05\x06\x00\x00\x00\x00"), want: domain.ArchiveZip, ok: true},
		{name: "7z", header: []byte("7z\xBC\xAF\x27\x1C\x00\x04"), want: domain.Archive7z, ok: true},
		{name: "rar 4", header: []byte("Rar!\x1A\x07\x00\xCF\x90"), want: domain.ArchiveRar, ok: true},
		{name: "rar 5", header: []byte("Rar!\x1A\x07\x01\x00"), want: domain.ArchiveRar, ok: true},
		{name: "matroska video", header: []byte("\x1A\x45\xDF\xA3\x93\x42\x82\x88"), ok: false},
		{name: "text renamed to zip", header: []byte("hello wor"), ok: false},
		{name: "too short", header: []byte("PK"), ok: false},
		{name: "nothing", header: nil, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := domain.DetectArchive(tt.header)
			if ok != tt.ok || got != tt.want {
				t.Errorf("DetectArchive = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
	if _, ok := domain.DetectArchive(bytes.Repeat([]byte{0}, 8)); ok {
		t.Error("zeros are not an archive")
	}
}

func TestParseVolumeRecognizesMultiVolumeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		file      string
		ok        bool
		index     int
		sameSetAs string
	}{
		{name: "rar part", file: "Movie.part1.rar", ok: true, index: 1, sameSetAs: "Movie.part2.rar"},
		{name: "rar part with padding", file: "movie.PART02.RAR", ok: true, index: 2, sameSetAs: "Movie.part1.rar"},
		{name: "7z split", file: "Movie.7z.001", ok: true, index: 1, sameSetAs: "Movie.7z.002"},
		{name: "zip split", file: "Movie.zip.003", ok: true, index: 3, sameSetAs: "Movie.zip.001"},
		{name: "another set", file: "Other.part1.rar", ok: true, index: 1},
		{name: "a plain archive", file: "Movie.7z"},
		{name: "a plain rar", file: "Movie.rar"},
		{name: "the old volume naming is not a set", file: "Movie.r00"},
		{name: "a zero index", file: "Movie.part0.rar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, ok := domain.ParseVolume(tt.file)
			if ok != tt.ok {
				t.Fatalf("ParseVolume(%q) ok = %v, want %v", tt.file, ok, tt.ok)
			}
			if !ok {
				return
			}
			if v.Index != tt.index {
				t.Errorf("index = %d, want %d", v.Index, tt.index)
			}
			if tt.sameSetAs != "" {
				other, _ := domain.ParseVolume(tt.sameSetAs)
				if other.Set != v.Set {
					t.Errorf("sets %q and %q differ", v.Set, other.Set)
				}
			}
		})
	}
}
