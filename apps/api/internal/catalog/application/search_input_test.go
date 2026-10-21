package application

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestParseTitleSearchID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		kind   domain.TitleKind
		tmdbID int64
		imdbID string
	}{
		{name: "numeric id", input: "273477", tmdbID: 273477},
		{name: "movie link", input: "https://www.themoviedb.org/movie/273477-scouts-guide?language=es-MX", kind: domain.KindMovie, tmdbID: 273477},
		{name: "series link with language and query", input: "https://themoviedb.org/es/tv/1396-breaking-bad?foo=bar", kind: domain.KindSeries, tmdbID: 1396},
		{name: "bare movie host", input: "themoviedb.org/movie/273477-scouts-guide", kind: domain.KindMovie, tmdbID: 273477},
		{name: "IMDb id", input: "tt0118583", imdbID: "tt0118583"},
		{name: "IMDb link", input: "https://www.imdb.com/title/tt0118583/?ref_=fn_al_tt_1", imdbID: "tt0118583"},
		{name: "legacy IMDb tag", input: "[imdbid-tt0118583]", imdbID: "tt0118583"},
		{name: "garbage", input: "not an id", tmdbID: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, tmdbID, imdbID := parseTitleSearchID(tt.input)
			if kind != tt.kind || tmdbID != tt.tmdbID || imdbID != tt.imdbID {
				t.Errorf("parseTitleSearchID(%q) = (%q, %d, %q), want (%q, %d, %q)", tt.input, kind, tmdbID, imdbID, tt.kind, tt.tmdbID, tt.imdbID)
			}
		})
	}
}
