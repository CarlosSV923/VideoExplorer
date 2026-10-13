package fileid

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

func TestLinksRefusesAPathThroughASymbolicLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Movies", "Real"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Movies", "Real", "a.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "Movies", "Linked")
	linkFile := filepath.Join(root, "Movies", "Real", "link.mkv")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.mkv"), linkFile); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}

	tests := []struct {
		name string
		path string
		// wantErr is the error NoLinks must return; nil means the path is accepted.
		wantErr error
		// outside marks a path that is refused for not being under the library.
		outside bool
	}{
		{name: "a regular file", path: filepath.Join(root, "Movies", "Real", "a.mkv")},
		{name: "a file that does not exist yet", path: filepath.Join(root, "Movies", "Real", "new.mkv")},
		{name: "a file below a missing folder", path: filepath.Join(root, "Movies", "Missing", "a.mkv")},
		{name: "a file inside a linked folder", path: filepath.Join(linkDir, "secret.mkv"), wantErr: application.ErrSymbolicLink},
		{name: "a linked file", path: linkFile, wantErr: application.ErrSymbolicLink},
		{name: "a file outside the library", path: filepath.Join(outside, "secret.mkv"), outside: true},
		{name: "a path that climbs out of the library", path: filepath.Join(root, "..", "x.mkv"), outside: true},
		{name: "the library folder itself", path: root, outside: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Links{}.NoLinks(root, tt.path)
			switch {
			case tt.outside:
				if err == nil || errors.Is(err, application.ErrSymbolicLink) {
					t.Fatalf("NoLinks = %v; want a refusal for a path outside the library", err)
				}
			case tt.wantErr == nil && err != nil:
				t.Fatalf("NoLinks = %v; want nil", err)
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Fatalf("NoLinks = %v; want %v", err, tt.wantErr)
			}
		})
	}
}
