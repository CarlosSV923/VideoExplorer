// Package ffprobe implements the Prober port with the ffprobe of jellyfin-ffmpeg
// (RNF-03a), run as a child process: the Go process never reads the video's bytes, only
// the small JSON that ffprobe prints about its streams (RNF-01).
package ffprobe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Binary is the ffprobe executable (installed by scripts/install-ffprobe.sh).
const Binary = "ffprobe"

// Timeout bounds one probe: a file that does not answer in this time is not read.
const Timeout = 30 * time.Second

// maxOutput bounds the JSON kept from ffprobe. A video's stream list is far smaller;
// anything beyond it is dropped, and the output then fails to parse.
const maxOutput = 4 << 20

// waitDelay is how long a killed ffprobe may keep its output pipes open before they are closed.
const waitDelay = 2 * time.Second

// Prober implements application.Prober.
type Prober struct {
	bin     string
	timeout time.Duration
}

var _ application.Prober = (*Prober)(nil)

// New builds a prober using bin (empty means "ffprobe" from PATH), with Timeout.
func New(bin string) *Prober {
	if bin == "" {
		bin = Binary
	}
	return &Prober{bin: bin, timeout: Timeout}
}

// Probe implements application.Prober. It runs ffprobe on the file and maps its JSON to
// the domain (RF-24, RF-44). A file ffprobe refuses fails with application.ErrNotVideo,
// carrying what ffprobe said; one that does not answer in time fails with
// application.ErrProbeTimeout. A cancelled ctx returns its error.
func (p *Prober) Probe(ctx context.Context, path string) (domain.MediaInfo, error) {
	if strings.HasPrefix(path, "-") {
		return domain.MediaInfo{}, errors.New("ffprobe: refusing a path that starts with a dash")
	}
	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(runCtx, p.bin, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path) //nolint:gosec // fixed binary; the path is built by the app from IDs
	cmd.Stdout = &limitedWriter{buf: &stdout, max: maxOutput}
	cmd.Stderr = &limitedWriter{buf: &stderr, max: maxOutput}
	// A killed probe whose pipes a child still holds is let go after waitDelay, not waited for.
	cmd.WaitDelay = waitDelay
	if err := cmd.Run(); err != nil {
		return domain.MediaInfo{}, classify(ctx, runCtx, err, stderr.String())
	}
	return ParseOutput(stdout.Bytes())
}

// classify maps the failure of one run. The caller's cancellation comes first, then the
// timeout (a killed process also reports an exit error), then ffprobe's refusal of the file.
func classify(parent, run context.Context, err error, stderr string) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(run.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w after %s", application.ErrProbeTimeout, Timeout)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%w: %s", application.ErrNotVideo, firstLine(stderr))
	}
	return fmt.Errorf("ffprobe: %w", err)
}

// firstLine returns the first non-empty line of the tool's output.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "no detail"
}

// limitedWriter keeps the first max bytes written and discards the rest without failing
// the write, so that a process printing too much never blocks on its pipe.
type limitedWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
