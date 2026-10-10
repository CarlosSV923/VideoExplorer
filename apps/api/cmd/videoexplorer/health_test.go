package main

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// The diagnostics read the folders of the configuration, in the order of RF-63.
// TMDB is never among them. The 7zz and ffprobe results depend on the host, so
// only their names are checked here (their own tests cover them).
func TestNewHealthChecksTheConfiguredFolders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		library       func(t *testing.T) string
		wantLibraryOK bool
	}{
		{
			name:          "writable library passes",
			library:       func(t *testing.T) string { return t.TempDir() },
			wantLibraryOK: true,
		},
		{
			name: "missing library fails with its path",
			library: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "library")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			library := tt.library(t)
			h := newHealth(config.Config{LibraryPath: library, DataPath: t.TempDir()})
			resp, err := h.GetHealth(t.Context(), httpapi.GetHealthRequestObject{})
			if err != nil {
				t.Fatalf("GetHealth: %v", err)
			}
			body := resp.(httpapi.GetHealth200JSONResponse).Body

			var names []string
			for _, c := range body.Checks {
				names = append(names, c.Name)
			}
			if want := []string{"library", "data", "7zz", "ffprobe"}; !slices.Equal(names, want) {
				t.Fatalf("checks = %v, want %v", names, want)
			}

			libCheck := body.Checks[0]
			if libCheck.Ok != tt.wantLibraryOK {
				t.Fatalf("library ok = %v, want %v: %+v", libCheck.Ok, tt.wantLibraryOK, libCheck)
			}
			if !tt.wantLibraryOK {
				if libCheck.Code == nil || *libCheck.Code != httpapi.LibraryNotWritable {
					t.Errorf("library code = %v, want library_not_writable", libCheck.Code)
				}
				if libCheck.Path == nil || *libCheck.Path != library {
					t.Errorf("library path = %v, want %q", libCheck.Path, library)
				}
			}
			if !body.Checks[1].Ok {
				t.Errorf("data check = %+v, want it to pass with a writable data folder", body.Checks[1])
			}
		})
	}
}
