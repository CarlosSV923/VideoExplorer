package sevenzip

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

func TestClassifyMapsTheToolsFailuresToTheirReasons(t *testing.T) {
	t.Parallel()

	exitErr := fmt.Errorf("exit status 2")
	tests := []struct {
		name     string
		err      error
		stderr   string
		password string
		want     error
	}{
		{
			name:   "disk full while writing (no_space)",
			err:    exitErr,
			stderr: "\nSystem ERROR:\nerrno=28 : No space left on device\n",
			want:   application.ErrNoSpace,
		},
		{
			name:     "wrong password",
			err:      exitErr,
			stderr:   "ERROR: Wrong password : Movie.mkv\n",
			password: "nope",
			want:     application.ErrWrongPassword,
		},
		{
			name:   "password needed",
			err:    exitErr,
			stderr: "ERROR: Cannot open encrypted archive. Wrong password?\n",
			want:   application.ErrPasswordRequired,
		},
		{
			name:   "damaged data (archive_corrupt)",
			err:    exitErr,
			stderr: "ERROR: Data Error : Movie.mkv\n",
			want:   application.ErrCorrupt,
		},
		{
			name: "the tool did not start",
			err:  errors.New(`exec: "7zz": executable file not found`),
			want: application.ErrCorrupt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classify(tt.err, tt.stderr, tt.password)
			if !errors.Is(got, tt.want) {
				t.Fatalf("classify = %v, want %v", got, tt.want)
			}
		})
	}
}

// A process that really exits with an error must still be classified from its
// stderr (not from the exit code alone).
func TestClassifyUsesStderrBeforeTheExitCode(t *testing.T) {
	t.Parallel()

	err := exec.CommandContext(t.Context(), "sh", "-c", "exit 2").Run()
	if got := classify(err, "No space left on device", ""); !errors.Is(got, application.ErrNoSpace) {
		t.Fatalf("classify = %v, want ErrNoSpace", got)
	}
}
