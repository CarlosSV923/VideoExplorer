package domain_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

type sanitizeVectors struct {
	Sanitize struct {
		Fallback string `json:"fallback"`
		Cases    []struct {
			Name  string `json:"name"`
			Input struct {
				Name string `json:"name"`
			} `json:"input"`
			Expected string `json:"expected"`
		} `json:"cases"`
	} `json:"sanitize"`
}

func loadSanitizeVectors(t *testing.T) sanitizeVectors {
	t.Helper()
	raw, err := os.ReadFile("../../../../../contracts/naming-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var v sanitizeVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Sanitize.Cases) == 0 || v.Sanitize.Fallback == "" {
		t.Fatal("no sanitize vectors loaded")
	}
	return v
}

func TestSanitizeNameGoldenVectors(t *testing.T) {
	t.Parallel()
	for _, c := range loadSanitizeVectors(t).Sanitize.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			if got := domain.SanitizeName(c.Input.Name); got != c.Expected {
				t.Errorf("SanitizeName(%q) = %q, want %q", c.Input.Name, got, c.Expected)
			}
		})
	}
}

func TestEntryNameUsesTheFallbackWhenNothingIsLeft(t *testing.T) {
	t.Parallel()

	fallback := loadSanitizeVectors(t).Sanitize.Fallback
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "a sanitized name is kept", raw: "Mission: Impossible", want: "Mission - Impossible"},
		{name: "an empty name gets the fallback", raw: "", want: fallback},
		{name: "a name of signs gets the fallback", raw: "?*|", want: fallback},
		{name: "a name of dots gets the fallback", raw: "..", want: fallback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.EntryName(tt.raw); got != tt.want {
				t.Errorf("EntryName(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestSanitizePathSanitizesEveryFolderAndTheFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rel  string
		want string
	}{
		{name: "a clean path is kept", rel: "Season 01/Pilot.mkv", want: "Season 01/Pilot.mkv"},
		{name: "a folder with a sign is sanitized", rel: "Season?/a.mkv", want: "Season/a.mkv"},
		{name: "a colon in a file name becomes a dash, the extension kept", rel: "Season 01/a:b.mkv", want: "Season 01/a-b.mkv"},
		{name: "the extension of a subtitle is kept, the stem sanitized", rel: "Sub/Movie.es.srt", want: "Sub/Movie.es.srt"},
		{name: "a folder starting with a dot is named without it", rel: ".hidden/a.mkv", want: "hidden/a.mkv"},
		{name: "a folder of dots takes the fallback name", rel: "x/.../a.mkv", want: "x/" + domain.FallbackName + "/a.mkv"},
		{name: "a file without extension is sanitized whole", rel: "Clip?", want: "Clip"},
		{name: "an empty path stays empty", rel: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.SanitizePath(tt.rel); got != tt.want {
				t.Errorf("SanitizePath(%q) = %q, want %q", tt.rel, got, tt.want)
			}
		})
	}
}
