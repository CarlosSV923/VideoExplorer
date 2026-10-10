// Package application holds the metadata use cases and the port they need.
package application

import (
	"context"
	"slices"
	"strings"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// Provider is the external metadata source (TMDB). Every call returns the text
// in the requested language; the language policy (RF-61) lives in Service.
type Provider interface {
	SearchMovies(ctx context.Context, language, query string, year *int) ([]domain.MovieSummary, error)
	SearchSeries(ctx context.Context, language, query string, year *int) ([]domain.SeriesSummary, error)
	// Movie and Series return domain.ErrNotFound for unknown ids.
	Movie(ctx context.Context, language string, id int64) (domain.Movie, error)
	Series(ctx context.Context, language string, id int64) (domain.Series, error)
	// Season returns domain.ErrNotFound when TMDB does not know the season.
	Season(ctx context.Context, language string, seriesID int64, number int) (domain.Season, error)
	Collection(ctx context.Context, language string, id int64) (domain.Collection, error)
	Genres(ctx context.Context, language string, kind domain.MediaKind) ([]domain.Genre, error)
}

// englishLanguage is the fallback of RF-61 for text TMDB does not have in the
// configured language.
const englishLanguage = "en-US"

// Service implements the metadata use cases. Text comes in the configured
// language (TMDB_LANGUAGE); where TMDB has no translation endpoint (seasons,
// collections, genres) the English text fills the fields left empty.
//
// A nil provider means no TMDB credentials: every call fails with
// domain.ErrNotConfigured and the app keeps working with free names (spec §6).
type Service struct {
	provider Provider
	language string
}

// NewService builds the service. provider may be nil; language is TMDB_LANGUAGE.
func NewService(provider Provider, language string) *Service {
	return &Service{provider: provider, language: language}
}

// Configured reports whether a provider is available.
func (s *Service) Configured() bool { return s.provider != nil }

// SearchMovies returns the movies that match the title, optionally from one year.
func (s *Service) SearchMovies(ctx context.Context, query string, year *int) ([]domain.MovieSummary, error) {
	q, err := s.searchInput(query, year)
	if err != nil {
		return nil, err
	}
	return s.provider.SearchMovies(ctx, s.language, q, year)
}

// SearchSeries returns the series that match the title, optionally from one year.
func (s *Service) SearchSeries(ctx context.Context, query string, year *int) ([]domain.SeriesSummary, error) {
	q, err := s.searchInput(query, year)
	if err != nil {
		return nil, err
	}
	return s.provider.SearchSeries(ctx, s.language, q, year)
}

// Movie returns one movie with its genres named in the configured language.
func (s *Service) Movie(ctx context.Context, id int64) (domain.Movie, error) {
	if err := s.ready(id); err != nil {
		return domain.Movie{}, err
	}
	m, err := s.provider.Movie(ctx, s.language, id)
	if err != nil {
		return domain.Movie{}, err
	}
	if m.Genres, err = s.fillGenreNames(ctx, domain.KindMovie, m.Genres); err != nil {
		return domain.Movie{}, err
	}
	return m, nil
}

// Series returns one series with its genres named in the configured language.
func (s *Service) Series(ctx context.Context, id int64) (domain.Series, error) {
	if err := s.ready(id); err != nil {
		return domain.Series{}, err
	}
	series, err := s.provider.Series(ctx, s.language, id)
	if err != nil {
		return domain.Series{}, err
	}
	if series.Genres, err = s.fillGenreNames(ctx, domain.KindSeries, series.Genres); err != nil {
		return domain.Series{}, err
	}
	return series, nil
}

// Season returns the episodes of one season. Episodes without a title or an
// overview in the configured language take the English text.
func (s *Service) Season(ctx context.Context, seriesID int64, number int) (domain.Season, error) {
	if err := s.ready(seriesID); err != nil {
		return domain.Season{}, err
	}
	season, err := s.provider.Season(ctx, s.language, seriesID, number)
	if err != nil {
		return domain.Season{}, err
	}
	if s.isEnglish() || !slices.ContainsFunc(season.Episodes, missingEpisodeText) {
		return season, nil
	}
	english, err := s.provider.Season(ctx, englishLanguage, seriesID, number)
	if err != nil {
		return domain.Season{}, err
	}
	byNumber := make(map[int]domain.Episode, len(english.Episodes))
	for _, e := range english.Episodes {
		byNumber[e.Number] = e
	}
	for i := range season.Episodes {
		ep := &season.Episodes[i]
		src, ok := byNumber[ep.Number]
		if !ok {
			continue
		}
		if ep.Title == "" {
			ep.Title = src.Title
		}
		if ep.Overview == "" {
			ep.Overview = src.Overview
		}
	}
	return season, nil
}

// Collection returns a collection. Its name and the titles of its movies take
// the English text where the configured language has none.
func (s *Service) Collection(ctx context.Context, id int64) (domain.Collection, error) {
	if err := s.ready(id); err != nil {
		return domain.Collection{}, err
	}
	c, err := s.provider.Collection(ctx, s.language, id)
	if err != nil {
		return domain.Collection{}, err
	}
	if s.isEnglish() || (c.Name != "" && !slices.ContainsFunc(c.Parts, func(p domain.CollectionPart) bool { return p.Title == "" })) {
		return c, nil
	}
	english, err := s.provider.Collection(ctx, englishLanguage, id)
	if err != nil {
		return domain.Collection{}, err
	}
	if c.Name == "" {
		c.Name = english.Name
	}
	if c.Overview == "" {
		c.Overview = english.Overview
	}
	titles := make(map[int64]string, len(english.Parts))
	for _, p := range english.Parts {
		titles[p.ID] = p.Title
	}
	for i := range c.Parts {
		if c.Parts[i].Title == "" {
			c.Parts[i].Title = titles[c.Parts[i].ID]
		}
	}
	return c, nil
}

// Genres returns the genre list of a kind, named in the configured language.
func (s *Service) Genres(ctx context.Context, kind domain.MediaKind) ([]domain.Genre, error) {
	if s.provider == nil {
		return nil, domain.ErrNotConfigured
	}
	genres, err := s.provider.Genres(ctx, s.language, kind)
	if err != nil {
		return nil, err
	}
	return s.fillGenreNames(ctx, kind, genres)
}

// fillGenreNames gives the English name to the genres left unnamed in the
// configured language. It asks TMDB for English only when one is missing.
func (s *Service) fillGenreNames(ctx context.Context, kind domain.MediaKind, genres []domain.Genre) ([]domain.Genre, error) {
	if s.isEnglish() || !slices.ContainsFunc(genres, func(g domain.Genre) bool { return g.Name == "" }) {
		return genres, nil
	}
	english, err := s.provider.Genres(ctx, englishLanguage, kind)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(english))
	for _, g := range english {
		names[g.ID] = g.Name
	}
	out := slices.Clone(genres)
	for i := range out {
		if out[i].Name == "" {
			out[i].Name = names[out[i].ID]
		}
	}
	return out, nil
}

// ready checks that a provider exists and the id is positive.
func (s *Service) ready(id int64) error {
	if s.provider == nil {
		return domain.ErrNotConfigured
	}
	if id <= 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Service) searchInput(query string, year *int) (string, error) {
	if s.provider == nil {
		return "", domain.ErrNotConfigured
	}
	if year != nil && *year <= 0 {
		return "", domain.ErrInvalidQuery
	}
	return domain.NormalizeQuery(query)
}

// isEnglish reports whether the configured language is English, where no fallback is needed.
func (s *Service) isEnglish() bool {
	lang, _, _ := strings.Cut(s.language, "-")
	return strings.EqualFold(lang, "en")
}

func missingEpisodeText(e domain.Episode) bool {
	return e.Title == "" || e.Overview == ""
}
