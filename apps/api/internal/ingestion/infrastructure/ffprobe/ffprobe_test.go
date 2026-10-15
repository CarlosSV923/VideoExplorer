package ffprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

// requireFFprobe skips without ffprobe in PATH, except where REQUIRE_FFPROBE is set
// (the dev image and CI), so these tests can never silently stop running there.
func requireFFprobe(t *testing.T) *Prober {
	t.Helper()

	if _, err := exec.LookPath(Binary); err != nil {
		if os.Getenv("REQUIRE_FFPROBE") != "" {
			t.Fatal("ffprobe not found but REQUIRE_FFPROBE is set")
		}
		t.Skip("ffprobe not installed")
	}
	return New("")
}

// slowBinary writes a program that runs for a long time, to test the timeout and the
// cancellation of a probe without a real slow file.
func slowBinary(t *testing.T) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the fake ffprobe is a shell script")
	}
	script := filepath.Join(t.TempDir(), "ffprobe")
	if err := writeProgram(script, []byte("#!/bin/sh\nexec sleep 30\n")); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestProbeReadsEveryFixtureAsItsExpectation(t *testing.T) {
	t.Parallel()

	p := requireFFprobe(t)
	for _, tt := range fixtures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := p.Probe(t.Context(), filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Probe = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestProbeRefusesWhatIsNotAVideo(t *testing.T) {
	t.Parallel()

	p := requireFFprobe(t)
	text := filepath.Join(t.TempDir(), "notes.mkv")
	if err := os.WriteFile(text, []byte("this is not a video, only text\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := p.Probe(t.Context(), text)
	if !errors.Is(err, application.ErrNotVideo) {
		t.Fatalf("err = %v, want ErrNotVideo", err)
	}
}

func TestProbeRefusesPathsThatLookLikeOptions(t *testing.T) {
	t.Parallel()

	p := New("")
	_, err := p.Probe(t.Context(), "-version")
	if err == nil || errors.Is(err, application.ErrNotVideo) {
		t.Fatalf("err = %v, want a refusal that is not a verdict on the file", err)
	}
}

func TestProbeGivesUpAfterItsTimeout(t *testing.T) {
	t.Parallel()

	p := &Prober{bin: slowBinary(t), timeout: 200 * time.Millisecond}
	start := time.Now()
	_, err := p.Probe(t.Context(), "video.mkv")
	if !errors.Is(err, application.ErrProbeTimeout) {
		t.Fatalf("err = %v, want ErrProbeTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("the probe took %s after a 200 ms timeout", elapsed)
	}
}

func TestProbeStopsWhenItsCallerCancels(t *testing.T) {
	t.Parallel()

	p := &Prober{bin: slowBinary(t), timeout: time.Minute}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	_, err := p.Probe(ctx, "video.mkv")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the caller's context error", err)
	}
	if errors.Is(err, application.ErrProbeTimeout) {
		t.Fatalf("err = %v: the caller's cancellation is not a probe timeout", err)
	}
}
