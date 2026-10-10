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
		original     string // default "Il Padrino", an Italian title that never matches the languages below
		origLanguage string // default "it"
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
			name:         "missing region uses another variant of the same language",
			translations: sampleTranslations,
			language:     "es-MX",
			wantTitle:    "El padrino",
			wantOverview: "Resumen en España",
		},
		{
			name:         "bare language uses a regional variant of it",
			translations: sampleTranslations,
			language:     "es",
			wantTitle:    "El padrino",
			wantOverview: "Resumen en España",
		},
		{
			name:         "no variant of the language falls back to English",
			translations: sampleTranslations,
			language:     "de-DE",
			wantTitle:    "The Godfather",
			wantOverview: "English overview",
		},
		{
			name: "same language with empty title moves on to the next variant of it",
			translations: []domain.Translation{
				{Language: "es-ES", Title: "", Overview: "Ver"},
				{Language: "es", Title: "El padrino", Overview: ""},
				{Language: "en-US", Title: "The Godfather", Overview: "English overview"},
			},
			language:     "es-MX",
			wantTitle:    "El padrino",
			wantOverview: "Ver",
		},
		{
			name: "same language overview is preferred over English",
			translations: []domain.Translation{
				{Language: "en-US", Title: "The Godfather", Overview: "English overview"},
				{Language: "es-AR", Title: "El padrino", Overview: "Resumen argentino"},
			},
			language:     "es-MX",
			wantTitle:    "El padrino",
			wantOverview: "Resumen argentino",
		},
		{
			name:         "English request uses its own variant before any other English",
			translations: []domain.Translation{{Language: "en-GB", Title: "The Godfather GB"}, {Language: "en-US", Title: "The Godfather"}},
			language:     "en-GB",
			wantTitle:    "The Godfather GB",
			wantOverview: "",
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
		{
			name:         "Spanish original is the Spanish title when TMDB has no Spanish one",
			original:     "La casa de papel",
			origLanguage: "es",
			translations: []domain.Translation{{Language: "en-US", Title: "Money Heist", Overview: "English overview"}},
			language:     "es-MX",
			wantTitle:    "La casa de papel",
			wantOverview: "English overview",
		},
		{
			name:         "Spanish original with a regional translation: the translation wins",
			original:     "La casa de papel",
			origLanguage: "es",
			translations: []domain.Translation{{Language: "es-ES", Title: "La casa de papel (ES)"}, {Language: "en-US", Title: "Money Heist"}},
			language:     "es-MX",
			wantTitle:    "La casa de papel (ES)",
			wantOverview: "",
		},
		{
			name:         "region-tagged original language counts by its language",
			original:     "La casa de papel",
			origLanguage: "es-ES",
			translations: []domain.Translation{{Language: "en-US", Title: "Money Heist"}},
			language:     "es-MX",
			wantTitle:    "La casa de papel",
			wantOverview: "",
		},
		{
			name:         "Spanish original is not a candidate for English",
			original:     "La casa de papel",
			origLanguage: "es",
			translations: []domain.Translation{{Language: "en-US", Title: "Money Heist"}},
			language:     "en-US",
			wantTitle:    "Money Heist",
			wantOverview: "",
		},
		{
			name:         "Spanish original is not a candidate for another language",
			original:     "La casa de papel",
			origLanguage: "es",
			translations: []domain.Translation{{Language: "en-US", Title: "Money Heist"}},
			language:     "fr-FR",
			wantTitle:    "Money Heist",
			wantOverview: "",
		},
		{
			name:         "Japanese original keeps the normal order for es-MX: English before the original",
			original:     "千と千尋の神隠し",
			origLanguage: "ja",
			translations: []domain.Translation{{Language: "en-US", Title: "Spirited Away"}},
			language:     "es-MX",
			wantTitle:    "Spirited Away",
			wantOverview: "",
		},
		{
			name:         "Japanese original without any translation is the last fallback",
			original:     "千と千尋の神隠し",
			origLanguage: "ja",
			language:     "es-MX",
			wantTitle:    "千と千尋の神隠し",
			wantOverview: "",
		},
		{
			name:         "no original language means the original is not a candidate",
			original:     "La casa de papel",
			origLanguage: "",
			translations: []domain.Translation{{Language: "en-US", Title: "Money Heist"}},
			language:     "es-MX",
			wantTitle:    "Money Heist",
			wantOverview: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original, origLanguage := tt.original, tt.origLanguage
			if original == "" {
				original, origLanguage = "Il Padrino", "it"
			}
			title, overview := domain.Localize(original, origLanguage, tt.translations, tt.language)
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
