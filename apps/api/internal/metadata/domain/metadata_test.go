package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

var sampleTranslations = []domain.Translation{
	{Language: "en-US", Title: "The Godfather", Overview: "English overview"},
	{Language: "es-ES", Title: "El padrino", Overview: "Resumen en España"},
	{Language: "fr-FR", Title: "Le Parrain", Overview: ""},
}

func TestLocalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		translations []domain.Translation
		language     string
		wantTitle    string
		wantOverview string
	}{
		{
			name:         "own translation wins",
			translations: sampleTranslations,
			language:     "es-ES",
			wantTitle:    "El padrino",
			wantOverview: "Resumen en España",
		},
		{
			name:         "missing language falls back to English",
			translations: sampleTranslations,
			language:     "es-MX",
			wantTitle:    "The Godfather",
			wantOverview: "English overview",
		},
		{
			name:         "empty overview in own language falls back to English",
			translations: sampleTranslations,
			language:     "fr-FR",
			wantTitle:    "Le Parrain",
			wantOverview: "English overview",
		},
		{
			name:         "no translations at all uses the original title and no overview",
			translations: nil,
			language:     "es-MX",
			wantTitle:    "Il Padrino",
			wantOverview: "",
		},
		{
			name:         "language match is case-insensitive",
			translations: sampleTranslations,
			language:     "ES-es",
			wantTitle:    "El padrino",
			wantOverview: "Resumen en España",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			title, overview := domain.Localize("Il Padrino", tt.translations, tt.language)
			if title != tt.wantTitle || overview != tt.wantOverview {
				t.Errorf("Localize = (%q, %q), want (%q, %q)", title, overview, tt.wantTitle, tt.wantOverview)
			}
		})
	}
}

func TestEnglishTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		translations []domain.Translation
		want         string
	}{
		{"en-US preferred", []domain.Translation{{Language: "en-GB", Title: "GB"}, {Language: "en-US", Title: "US"}}, "US"},
		{"other English variant", []domain.Translation{{Language: "en-GB", Title: "GB"}}, "GB"},
		{"no English title", []domain.Translation{{Language: "es-ES", Title: "Español"}}, ""},
		{"English entry without title", []domain.Translation{{Language: "en-US", Overview: "only overview"}}, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := domain.EnglishTitle(tt.translations); got != tt.want {
				t.Errorf("EnglishTitle = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in     string
		wantOK bool
		year   int
	}{
		{"2008-01-20", true, 2008},
		{"", false, 0},
		{"2008", false, 0},
		{"2008-13-01", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, ok := domain.ParseDate(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ParseDate(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if ok && got.Year() != tt.year {
				t.Errorf("year = %d, want %d", got.Year(), tt.year)
			}
		})
	}
}

func TestYearOf(t *testing.T) {
	t.Parallel()

	if got := domain.YearOf(time.Time{}); got != nil {
		t.Errorf("YearOf(zero) = %d, want nil", *got)
	}
	date := time.Date(1972, time.March, 24, 0, 0, 0, 0, time.UTC)
	if got := domain.YearOf(date); got == nil || *got != 1972 {
		t.Errorf("YearOf = %v, want 1972", got)
	}
}

func TestNormalizeQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"collapses spaces", "  Iron   Man ", "Iron Man", nil},
		{"single letter is allowed", "V", "V", nil},
		{"empty", "   ", "", domain.ErrInvalidQuery},
		{"overlong", strings.Repeat("a", 201), "", domain.ErrInvalidQuery},
		{"200 runes is allowed", strings.Repeat("ñ", 200), strings.Repeat("ñ", 200), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NormalizeQuery(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("NormalizeQuery = %q, want %q", got, tt.want)
			}
		})
	}
}
