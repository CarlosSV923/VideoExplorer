// Package application holds the metadata use cases and the port they need.
package application

import (
	"context"
	"errors"
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

// englishLanguage is the last fallback of RF-61 for text TMDB does not have in
// the configured language.
const englishLanguage = "en-US"

// Service implements the metadata use cases. Text comes in the configured
// language (TMDB_LANGUAGE). Movies and series get their translations from
// TMDB's translations endpoints, so the domain resolves every variant at once.
// Seasons, collections and genres have no such endpoint, so for them the
// service asks, one request per list, for the fallback languages (see
// fallbackLanguages) and fills the fields left empty.
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
// Titles are the ones TMDB returns in the configured language (RF-61).
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
// overview in the configured language take the text of the fallback languages.
func (s *Service) Season(ctx context.Context, seriesID int64, number int) (domain.Season, error) {
	if err := s.ready(seriesID); err != nil {
		return domain.Season{}, err
	}
	season, err := s.provider.Season(ctx, s.language, seriesID, number)
	if err != nil {
		return domain.Season{}, err
	}
	for _, language := range s.fallbackLanguages() {
		if !slices.ContainsFunc(season.Episodes, missingEpisodeText) {
			break
		}
		src, err := s.provider.Season(ctx, language, seriesID, number)
		if err != nil {
			if skipFallback(err) {
				continue
			}
			return domain.Season{}, err
		}
		fillEpisodes(season.Episodes, src.Episodes)
	}
	return season, nil
}

// Collection returns a collection. Its name, overview and the titles of its
// movies take the text of the fallback languages where the configured one has none.
func (s *Service) Collection(ctx context.Context, id int64) (domain.Collection, error) {
	if err := s.ready(id); err != nil {
		return domain.Collection{}, err
	}
	c, err := s.provider.Collection(ctx, s.language, id)
	if err != nil {
		return domain.Collection{}, err
	}
	for _, language := range s.fallbackLanguages() {
		if !missingCollectionText(c) {
			break
		}
		src, err := s.provider.Collection(ctx, language, id)
		if err != nil {
			if skipFallback(err) {
				continue
			}
			return domain.Collection{}, err
		}
		fillCollection(&c, src)
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

// fillGenreNames names the genres left unnamed in the configured language, one
// request per fallback language and only while some name is still missing.
func (s *Service) fillGenreNames(ctx context.Context, kind domain.MediaKind, genres []domain.Genre) ([]domain.Genre, error) {
	out := slices.Clone(genres)
	for _, language := range s.fallbackLanguages() {
		if !slices.ContainsFunc(out, func(g domain.Genre) bool { return g.Name == "" }) {
			break
		}
		src, err := s.provider.Genres(ctx, language, kind)
		if err != nil {
			if skipFallback(err) {
				continue
			}
			return nil, err
		}
		names := make(map[int64]string, len(src))
		for _, g := range src {
			names[g.ID] = g.Name
		}
		for i := range out {
			if out[i].Name == "" {
				out[i].Name = names[out[i].ID]
			}
		}
	}
	return out, nil
}

// fallbackLanguages lists, in order, the languages tried after the configured
// one when it has no text: the bare language of a regional tag (es-MX -> es),
// then en-US. English needs no fallback. Each entry costs one request per list
// (season, collection, genre list), never one per element.
func (s *Service) fallbackLanguages() []string {
	if s.isEnglish() {
		return nil
	}
	primary, _, hasRegion := strings.Cut(s.language, "-")
	var out []string
	if hasRegion {
		out = append(out, strings.ToLower(primary))
	}
	return append(out, englishLanguage)
}

// skipFallback reports whether a failed fallback request should be ignored:
// TMDB does not know the list in that language, or it failed. The configured
// language already gave the primary text, so the field just stays empty.
func skipFallback(err error) bool {
	return errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUpstream)
}

// fillEpisodes copies the title and overview of the matching episodes of src
// into the empty fields of dst. Episodes are matched by number.
func fillEpisodes(dst, src []domain.Episode) {
	byNumber := make(map[int]domain.Episode, len(src))
	for _, e := range src {
		byNumber[e.Number] = e
	}
	for i := range dst {
		from, ok := byNumber[dst[i].Number]
		if !ok {
			continue
		}
		if dst[i].Title == "" {
			dst[i].Title = from.Title
		}
		if dst[i].Overview == "" {
			dst[i].Overview = from.Overview
		}
	}
}

// missingCollectionText reports whether a collection lacks a name or a movie title.
func missingCollectionText(c domain.Collection) bool {
	return c.Name == "" || slices.ContainsFunc(c.Parts, func(p domain.CollectionPart) bool { return p.Title == "" })
}

// fillCollection copies the empty name, overview and movie titles of src into c.
func fillCollection(c *domain.Collection, src domain.Collection) {
	if c.Name == "" {
		c.Name = src.Name
	}
	if c.Overview == "" {
		c.Overview = src.Overview
	}
	titles := make(map[int64]string, len(src.Parts))
	for _, p := range src.Parts {
		titles[p.ID] = p.Title
	}
	for i := range c.Parts {
		if c.Parts[i].Title == "" {
			c.Parts[i].Title = titles[c.Parts[i].ID]
		}
	}
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
