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
