package system_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

func TestWritableDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dir    func(t *testing.T) string
		wantOK bool
	}{
		{
			name:   "writable folder passes",
			dir:    func(t *testing.T) string { return t.TempDir() },
			wantOK: true,
		},
		{
			name: "missing folder fails",
			dir: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
		},
		{
			name: "path below a regular file fails",
			dir: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(file, "library")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.dir(t)
			failure := system.WritableDir("library", httpapi.LibraryNotWritable, dir).Run(t.Context())

			if tt.wantOK {
				if failure != nil {
					t.Fatalf("failed: %s", failure.Message)
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Errorf("the check left %d file(s) behind", len(entries))
				}
				return
			}

			if failure == nil {
				t.Fatal("passed, want a failure")
			}
			if failure.Path != filepath.Clean(dir) {
				t.Errorf("path = %q, want %q", failure.Path, filepath.Clean(dir))
			}
			if failure.UID == nil || *failure.UID != os.Getuid() || failure.GID == nil || *failure.GID != os.Getgid() {
				t.Errorf("uid/gid = %v/%v, want %d/%d", failure.UID, failure.GID, os.Getuid(), os.Getgid())
			}
			for _, want := range []string{"uid=", "gid=", "Modify", "ACL"} {
				if !strings.Contains(failure.Message, want) {
					t.Errorf("message %q does not mention %q", failure.Message, want)
				}
			}
		})
	}
}

func TestWritableDirRefusesAReadOnlyFolder(t *testing.T) {
	t.Parallel()

	// Root writes through permission bits, so this case needs a regular user.
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not stop it")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if failure := system.WritableDir("data", httpapi.DataNotWritable, dir).Run(t.Context()); failure == nil {
		t.Fatal("read-only folder passed, want a failure")
	}
}

// fakeRunner stands in for os/exec. found is the path LookPath returns (empty
// when the program is missing); out and err are what the program produces.
type fakeRunner struct {
	found   string
	out     []byte
	err     error
	gotPath string
	gotArgs []string
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.found == "" {
		return "", fmt.Errorf("exec: %q: executable file not found in $PATH", file)
	}
	return f.found, nil
}

func (f *fakeRunner) Output(_ context.Context, path string, args ...string) ([]byte, error) {
	f.gotPath, f.gotArgs = path, args
	return f.out, f.err
}

func TestTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		runner      *fakeRunner
		wantOK      bool
		wantMessage string
	}{
		{
			name:   "program in the PATH that starts passes",
			runner: &fakeRunner{found: "/usr/local/bin/ffprobe", out: []byte("ffprobe version 7.1")},
			wantOK: true,
		},
		{
			name:        "program missing from the PATH fails",
			runner:      &fakeRunner{},
			wantMessage: "ffprobe is not in PATH",
		},
		{
			name: "program that exits with an error fails and quotes its output",
			runner: &fakeRunner{
				found: "/usr/local/bin/ffprobe",
				out:   []byte("error while loading shared libraries: libx.so\nsecond line"),
				err:   errors.New("exit status 127"),
			},
			wantMessage: "ffprobe -version did not start: exit status 127. Output: error while loading shared libraries: libx.so",
		},
		{
			name:        "failure without output says so",
			runner:      &fakeRunner{found: "/usr/local/bin/ffprobe", err: errors.New("signal: killed")},
			wantMessage: "Output: no output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			check := system.Tool("ffprobe", httpapi.FfprobeMissing, tt.runner, "ffprobe", "-version")
			failure := check.Run(t.Context())

			if tt.wantOK {
				if failure != nil {
					t.Fatalf("failed: %s", failure.Message)
				}
				if tt.runner.gotPath != tt.runner.found || !slices.Equal(tt.runner.gotArgs, []string{"-version"}) {
					t.Errorf("ran %q %v, want %q [-version]", tt.runner.gotPath, tt.runner.gotArgs, tt.runner.found)
				}
				return
			}

			if failure == nil {
				t.Fatal("passed, want a failure")
			}
			if !strings.Contains(failure.Message, tt.wantMessage) {
				t.Errorf("message = %q, want it to contain %q", failure.Message, tt.wantMessage)
			}
			if failure.Path != "" || failure.UID != nil || failure.GID != nil {
				t.Errorf("a tool failure carries folder data: %+v", failure)
			}
		})
	}
}

// TestToolsStartWithTheRealBinaries runs the production checks against the
// installed 7zz and ffprobe. Without them it skips, unless REQUIRE_7ZZ or
// REQUIRE_FFPROBE is set (the dev image and CI set them): then it fails.
func TestToolsStartWithTheRealBinaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		program string
		args    []string
		code    httpapi.CheckCode
		require string
	}{
		{name: "7zz", program: "7zz", args: []string{"i"}, code: httpapi.SevenzipMissing, require: "REQUIRE_7ZZ"},
		{name: "ffprobe", program: "ffprobe", args: []string{"-version"}, code: httpapi.FfprobeMissing, require: "REQUIRE_FFPROBE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := exec.LookPath(tt.program); err != nil {
				if os.Getenv(tt.require) != "" {
					t.Fatalf("%s not found but %s is set: %v", tt.program, tt.require, err)
				}
				t.Skipf("%s is not installed; set %s to make this a failure", tt.program, tt.require)
			}
			check := system.Tool(tt.name, tt.code, system.ExecRunner{}, tt.program, tt.args...)
			if failure := check.Run(t.Context()); failure != nil {
				t.Fatalf("%s does not pass: %s", tt.program, failure.Message)
			}
		})
	}
}
