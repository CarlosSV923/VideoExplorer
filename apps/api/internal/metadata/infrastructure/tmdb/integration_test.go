package tmdb_test

import (
	"os"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/tmdb"
)

// TestRealTMDB calls the real TMDB API. It runs only when a developer sets
// TMDB_READ_TOKEN or TMDB_API_KEY, so CI (which has no key) skips it. The key
// is never printed.
func TestRealTMDB(t *testing.T) {
	token, hasToken := os.LookupEnv("TMDB_READ_TOKEN")
	key, hasKey := os.LookupEnv("TMDB_API_KEY")
	if (!hasToken || token == "") && (!hasKey || key == "") {
		t.Skip("set TMDB_READ_TOKEN or TMDB_API_KEY to run the real TMDB test")
	}
	client, err := tmdb.New(tmdb.Config{ReadToken: token, APIKey: key})
	if err != nil {
		t.Skipf("credentials: %v", err)
	}

	year := 2008
	hits, err := client.SearchMovies(t.Context(), "en-US", "Iron Man", &year)
	if err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no results for Iron Man (2008)")
	}

	m, err := client.Movie(t.Context(), "en-US", 1726)
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	if m.OriginalTitle != "Iron Man" || m.Title == "" || m.Year == nil || *m.Year != 2008 {
		t.Errorf("Movie(1726) = %+v", m)
	}

	series, err := client.Series(t.Context(), "en-US", 1396)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if series.OriginalTitle != "Breaking Bad" || len(series.Seasons) == 0 {
		t.Errorf("Series(1396) = %+v", series.OriginalTitle)
	}

	season, err := client.Season(t.Context(), "en-US", 1396, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if len(season.Episodes) == 0 || season.Episodes[0].Title == "" {
		t.Errorf("Season(1396, 1) has no titled episodes")
	}
}
