package domain_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

func TestPickLogo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		logos    []domain.Logo
		language string
		wantPath string // "" when no logo is expected
	}{
		{
			name: "same language beats a better rated English logo",
			logos: []domain.Logo{
				{Path: "/en.png", Language: "en", VoteAverage: 9, VoteCount: 100},
				{Path: "/es.png", Language: "es", VoteAverage: 5, VoteCount: 1},
			},
			language: "es-MX",
			wantPath: "/es.png",
		},
		{
			name: "same language beats a text-less logo",
			logos: []domain.Logo{
				{Path: "/none.png", Language: "", VoteAverage: 10, VoteCount: 500},
				{Path: "/es.png", Language: "es", VoteAverage: 1, VoteCount: 1},
			},
			language: "es-MX",
			wantPath: "/es.png",
		},
		{
			name: "exact tag group is the same as same-language for a bare request",
			logos: []domain.Logo{
				{Path: "/en.png", Language: "en", VoteAverage: 9},
				{Path: "/es.png", Language: "es", VoteAverage: 5},
			},
			language: "es",
			wantPath: "/es.png",
		},
		{
			name: "best rated logo inside the same-language group",
			logos: []domain.Logo{
				{Path: "/a.png", Language: "es", VoteAverage: 5, VoteCount: 10},
				{Path: "/b.png", Language: "es", VoteAverage: 6, VoteCount: 1},
			},
			language: "es-MX",
			wantPath: "/b.png",
		},
		{
			name: "English before text-less when no language matches",
			logos: []domain.Logo{
				{Path: "/none.png", Language: "", VoteAverage: 10, VoteCount: 500},
				{Path: "/en.png", Language: "en", VoteAverage: 1, VoteCount: 1},
			},
			language: "es-MX",
			wantPath: "/en.png",
		},
		{
			name: "English request uses the English group",
			logos: []domain.Logo{
				{Path: "/none.png", Language: "", VoteAverage: 10},
				{Path: "/en.png", Language: "en", VoteAverage: 2},
			},
			language: "en-US",
			wantPath: "/en.png",
		},
		{
			name: "text-less logo when there is no logo in the language or in English",
			logos: []domain.Logo{
				{Path: "/fr.png", Language: "fr", VoteAverage: 10},
				{Path: "/none.png", Language: "", VoteAverage: 1},
			},
			language: "es-MX",
			wantPath: "/none.png",
		},
		{
			name: "SVG logo in the request language is skipped for a raster one",
			logos: []domain.Logo{
				{Path: "/es.svg", Language: "es", VoteAverage: 10, VoteCount: 500},
				{Path: "/es.png", Language: "es", VoteAverage: 1, VoteCount: 1},
			},
			language: "es-MX",
			wantPath: "/es.png",
		},
		{
			name: "SVG logos are never chosen, even when they are the only ones",
			logos: []domain.Logo{
				{Path: "/none.svg", Language: "", VoteAverage: 10, VoteCount: 500},
				{Path: "/en.svg", Language: "en", VoteAverage: 9, VoteCount: 500},
			},
			language: "es-MX",
			wantPath: "",
		},
		{
			name: "a path that is not a plain image name is never chosen",
			logos: []domain.Logo{
				{Path: "/a b.png", Language: "es", VoteAverage: 10, VoteCount: 500},
				{Path: "../x.png", Language: "es", VoteAverage: 9, VoteCount: 500},
			},
			language: "es-MX",
			wantPath: "",
		},
		{
			name: "logos in other languages are never chosen",
			logos: []domain.Logo{
				{Path: "/fr.png", Language: "fr", VoteAverage: 10},
				{Path: "/de.png", Language: "de", VoteAverage: 9},
			},
			language: "es-MX",
			wantPath: "",
		},
		{
			name:     "no logos",
			logos:    nil,
			language: "es-MX",
			wantPath: "",
		},
		{
			name: "equal average goes to the most voted",
			logos: []domain.Logo{
				{Path: "/a.png", Language: "es", VoteAverage: 7, VoteCount: 3},
				{Path: "/b.png", Language: "es", VoteAverage: 7, VoteCount: 30},
			},
			language: "es-MX",
			wantPath: "/b.png",
		},
		{
			name: "equal average and votes go to the lower path",
			logos: []domain.Logo{
				{Path: "/z.png", Language: "es", VoteAverage: 7, VoteCount: 3},
				{Path: "/a.png", Language: "es", VoteAverage: 7, VoteCount: 3},
			},
			language: "es-MX",
			wantPath: "/a.png",
		},
		{
			name: "logo without path is ignored",
			logos: []domain.Logo{
				{Path: "", Language: "es", VoteAverage: 10},
				{Path: "/en.png", Language: "en", VoteAverage: 1},
			},
			language: "es-MX",
			wantPath: "/en.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := domain.PickLogo(tt.logos, tt.language)
			if ok != (tt.wantPath != "") || got.Path != tt.wantPath {
				t.Errorf("PickLogo = (%q, %v), want (%q, %v)", got.Path, ok, tt.wantPath, tt.wantPath != "")
			}
		})
	}
}
