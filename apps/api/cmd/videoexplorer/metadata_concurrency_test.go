package main

import (
	"context"
	"sync"
	"testing"
	"time"

	catalogdomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	metadataapp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	metadatadomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

type concurrentSeasonProvider struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	active  int
	maximum int
}

func (p *concurrentSeasonProvider) SearchMovies(context.Context, string, string, *int) ([]metadatadomain.MovieSummary, error) {
	return nil, nil
}

func (p *concurrentSeasonProvider) SearchSeries(context.Context, string, string, *int) ([]metadatadomain.SeriesSummary, error) {
	return nil, nil
}

func (p *concurrentSeasonProvider) FindByIMDb(context.Context, string, string) (metadatadomain.FindResult, error) {
	return metadatadomain.FindResult{}, nil
}

func (p *concurrentSeasonProvider) Movie(context.Context, string, int64) (metadatadomain.Movie, error) {
	return metadatadomain.Movie{}, nil
}

func (p *concurrentSeasonProvider) Series(context.Context, string, int64) (metadatadomain.Series, error) {
	return metadatadomain.Series{
		ID: 9, Title: "Series", OriginalTitle: "Series", OriginalLanguage: "en",
		Seasons: []metadatadomain.SeasonSummary{{Number: 1}, {Number: 2}},
	}, nil
}

func (p *concurrentSeasonProvider) Season(ctx context.Context, language string, seriesID int64, number int) (metadatadomain.Season, error) {
	if language == "es-MX" {
		p.mu.Lock()
		p.active++
		if p.active > p.maximum {
			p.maximum = p.active
		}
		p.mu.Unlock()
		p.started <- struct{}{}
		select {
		case <-p.release:
		case <-ctx.Done():
			return metadatadomain.Season{}, ctx.Err()
		}
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}
	return metadatadomain.Season{SeriesID: seriesID, Number: number, Episodes: []metadatadomain.Episode{{
		ID: int64(number), Number: 1, Title: "Episode", Overview: "Overview",
	}}}, nil
}

func (p *concurrentSeasonProvider) Collection(context.Context, string, int64) (metadatadomain.Collection, error) {
	return metadatadomain.Collection{}, nil
}

func (p *concurrentSeasonProvider) Genres(context.Context, string, metadatadomain.MediaKind) ([]metadatadomain.Genre, error) {
	return nil, nil
}

func TestSeriesDetailsLoadsSeasonsConcurrently(t *testing.T) {
	t.Parallel()
	provider := &concurrentSeasonProvider{started: make(chan struct{}, 2), release: make(chan struct{})}
	adapter := tmdbOf{svc: metadataapp.NewService(provider, "es-MX")}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, err := adapter.Details(ctx, catalogdomain.KindSeries, 9)
		result <- err
	}()
	for range 2 {
		select {
		case <-provider.started:
		case <-ctx.Done():
			t.Fatal("season requests did not run concurrently")
		}
	}
	close(provider.release)
	if err := <-result; err != nil {
		t.Fatalf("Details: %v", err)
	}
	provider.mu.Lock()
	maximum := provider.maximum
	provider.mu.Unlock()
	if maximum < 2 {
		t.Fatalf("maximum simultaneous season requests = %d, want at least 2", maximum)
	}
}
