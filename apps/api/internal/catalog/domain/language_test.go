package domain_test

import (
	"errors"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

type languageVectors struct {
	SubtitleLanguage struct {
		Cases []struct {
			Name  string `json:"name"`
			Input struct {
				Language string `json:"language"`
			} `json:"input"`
			Expected expectation `json:"expected"`
		} `json:"cases"`
	} `json:"subtitleLanguage"`
}

func TestNormalizeLanguageGoldenVectors(t *testing.T) {
	t.Parallel()

	var v languageVectors
	readVectors(t, &v)
	if len(v.SubtitleLanguage.Cases) == 0 {
		t.Fatal("no subtitle language vectors loaded")
	}
	for _, c := range v.SubtitleLanguage.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.NormalizeLanguage(c.Input.Language)
			if c.Expected.Error != "" {
				var nameErr *domain.NameError
				if !errors.As(err, &nameErr) || nameErr.Field != c.Expected.Error {
					t.Fatalf("NormalizeLanguage(%q) error = %v, want a NameError for %q", c.Input.Language, err, c.Expected.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeLanguage(%q): %v", c.Input.Language, err)
			}
			if got != c.Expected.Value {
				t.Errorf("NormalizeLanguage(%q) = %q, want %q", c.Input.Language, got, c.Expected.Value)
			}
		})
	}
}
