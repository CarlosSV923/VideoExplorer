package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
)

func TestEnsureUnassignedDirCreatesExpectedFolderAndAcceptsExistingDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := ensureUnassignedDir(root); err != nil {
		t.Fatal(err)
	}
	if err := ensureUnassignedDir(root); err != nil {
		t.Fatalf("existing directory: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "_unassigned"))
	if err != nil || !info.IsDir() {
		t.Fatalf("_unassigned = %v, err = %v", info, err)
	}
}

func TestHashPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "line with a newline", input: "pw\n"},
		{name: "no trailing newline", input: "pw"},
		{name: "windows line ending", input: "pw\r\n"},
		{name: "empty line", input: "\n", wantErr: true},
		{name: "nothing on stdin", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			err := hashPassword(strings.NewReader(tt.input), &out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			v, err := identityinfra.NewArgon2idVerifier(strings.TrimSpace(out.String()))
			if err != nil {
				t.Fatalf("printed hash is not a valid argon2id PHC string: %v", err)
			}
			if !v.Verify("pw") || v.Verify("pw ") {
				t.Fatal("printed hash does not verify the password read from stdin")
			}
		})
	}
}

func TestDataPathWritable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     func(t *testing.T) string
		wantOK  bool
		wantLog bool
	}{
		{
			name:   "writable folder passes and logs nothing",
			dir:    func(t *testing.T) string { return t.TempDir() },
			wantOK: true,
		},
		{
			name: "missing folder stops the start",
			dir: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
			wantLog: true,
		},
		{
			name: "path below a regular file stops the start",
			dir: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(file, "data")
			},
			wantLog: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.dir(t)
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, nil))

			if ok := dataPathWritable(dir, log); ok != tt.wantOK {
				t.Fatalf("writable = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantLog {
				if buf.Len() != 0 {
					t.Fatalf("logged %q for a writable folder", buf.String())
				}
				return
			}

			var rec map[string]any
			if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
				t.Fatalf("log is not one JSON record: %v: %q", err, buf.String())
			}
			if rec["level"] != "ERROR" {
				t.Errorf("level = %v, want ERROR", rec["level"])
			}
			if rec["path"] != filepath.Clean(dir) {
				t.Errorf("path = %v, want %q", rec["path"], filepath.Clean(dir))
			}
			if rec["uid"] != float64(os.Getuid()) || rec["gid"] != float64(os.Getgid()) {
				t.Errorf("uid/gid = %v/%v, want %d/%d", rec["uid"], rec["gid"], os.Getuid(), os.Getgid())
			}
			msg, _ := rec["error"].(string)
			for _, want := range []string{"Modify", "ACL"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not mention %q", msg, want)
				}
			}
		})
	}
}

func TestDataPathWritableRefusesAReadOnlyFolder(t *testing.T) {
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

	var buf bytes.Buffer
	if dataPathWritable(dir, slog.New(slog.NewJSONHandler(&buf, nil))) {
		t.Fatal("read-only folder passed, want the start to stop")
	}
}
