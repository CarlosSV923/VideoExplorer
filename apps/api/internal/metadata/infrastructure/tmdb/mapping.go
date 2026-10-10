package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// The endpoints below are the TMDB v3 calls the app uses. Titles and overviews
// come from the translations endpoints, because the append_to_response
// behaviour for them is not documented; image languages go in
// include_image_language.

// ---------- search ----------

type apiSearchMovie struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	OriginalTitle    string `json:"original_title"`
	OriginalLanguage string `json:"original_language"`
	ReleaseDate      string `json:"release_date"`
	PosterPath       string `json:"poster_path"`
}

type apiSearchSeries struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	OriginalLanguage string `json:"original_language"`
	FirstAirDate     string `json:"first_air_date"`
	PosterPath       string `json:"poster_path"`
}

// SearchMovies returns the first page of TMDB's movie search. The app does not
// page through results: the first page holds the best matches.
func (c *Client) SearchMovies(ctx context.Context, language, query string, year *int) ([]domain.MovieSummary, error) {
	q := url.Values{"query": {query}, "language": {language}}
	if year != nil {
		q.Set("year", strconv.Itoa(*year))
	}
	var raw struct {
		Results []apiSearchMovie `json:"results"`
	}
	if err := c.get(ctx, "/search/movie", q, &raw); err != nil {
		return nil, err
	}
	out := make([]domain.MovieSummary, 0, len(raw.Results))
	for _, r := range raw.Results {
		if r.ID <= 0 {
			return nil, c.malformed("/search/movie", "id")
		}
		out = append(out, domain.MovieSummary{
			ID:               r.ID,
			Title:            firstNonEmpty(r.Title, r.OriginalTitle),
			OriginalTitle:    r.OriginalTitle,
			OriginalLanguage: r.OriginalLanguage,
			Year:             domain.YearOf(parseDate(r.ReleaseDate)),
			PosterPath:       validPath(r.PosterPath),
		})
	}
	return out, nil
}

// SearchSeries returns the first page of TMDB's series search.
func (c *Client) SearchSeries(ctx context.Context, language, query string, year *int) ([]domain.SeriesSummary, error) {
	q := url.Values{"query": {query}, "language": {language}}
	if year != nil {
		q.Set("first_air_date_year", strconv.Itoa(*year))
	}
	var raw struct {
		Results []apiSearchSeries `json:"results"`
	}
	if err := c.get(ctx, "/search/tv", q, &raw); err != nil {
		return nil, err
	}
	out := make([]domain.SeriesSummary, 0, len(raw.Results))
	for _, r := range raw.Results {
		if r.ID <= 0 {
			return nil, c.malformed("/search/tv", "id")
		}
		out = append(out, domain.SeriesSummary{
			ID:               r.ID,
			Title:            firstNonEmpty(r.Name, r.OriginalName),
			OriginalTitle:    r.OriginalName,
			OriginalLanguage: r.OriginalLanguage,
			Year:             domain.YearOf(parseDate(r.FirstAirDate)),
			PosterPath:       validPath(r.PosterPath),
		})
	}
	return out, nil
}

// ---------- movies ----------

type apiGenre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type apiCollectionRef struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PosterPath string `json:"poster_path"`
}

type apiMovie struct {
	ID               int64             `json:"id"`
	OriginalTitle    string            `json:"original_title"`
	OriginalLanguage string            `json:"original_language"`
	ReleaseDate      string            `json:"release_date"`
	Runtime          int               `json:"runtime"`
	PosterPath       string            `json:"poster_path"`
	BackdropPath     string            `json:"backdrop_path"`
	Genres           []apiGenre        `json:"genres"`
	Collection       *apiCollectionRef `json:"belongs_to_collection"`
}

// Movie returns one movie with its title and overview in language (English
// where the language has none), its genres and its logos.
func (c *Client) Movie(ctx context.Context, language string, id int64) (domain.Movie, error) {
	if id <= 0 {
		return domain.Movie{}, domain.ErrNotFound
	}
	base := "/movie/" + strconv.FormatInt(id, 10)
	var raw apiMovie
	if err := c.get(ctx, base, url.Values{"language": {language}}, &raw); err != nil {
		return domain.Movie{}, err
	}
	if raw.ID <= 0 {
		return domain.Movie{}, c.malformed(base, "id")
	}
	translations, err := c.translations(ctx, base+"/translations")
	if err != nil {
		return domain.Movie{}, err
	}
	logos, err := c.logos(ctx, base+"/images", language)
	if err != nil {
		return domain.Movie{}, err
	}
	title, overview := domain.Localize(raw.OriginalTitle, translations, language)

	m := domain.Movie{
		ID:               raw.ID,
		Title:            title,
		EnglishTitle:     domain.EnglishTitle(translations),
		OriginalTitle:    raw.OriginalTitle,
		OriginalLanguage: raw.OriginalLanguage,
		Overview:         overview,
		Year:             domain.YearOf(parseDate(raw.ReleaseDate)),
		Runtime:          max(raw.Runtime, 0),
		Genres:           toGenres(raw.Genres),
		Images: domain.Images{
			Poster:   validPath(raw.PosterPath),
			Backdrop: validPath(raw.BackdropPath),
			Logos:    logos,
		},
	}
	if raw.Collection != nil && raw.Collection.ID > 0 {
		m.Collection = &domain.CollectionRef{
			ID:         raw.Collection.ID,
			Name:       raw.Collection.Name,
			PosterPath: validPath(raw.Collection.PosterPath),
		}
	}
	return m, nil
}

// ---------- series ----------

type apiSeasonSummary struct {
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date"`
	PosterPath   string `json:"poster_path"`
}

type apiSeries struct {
	ID               int64              `json:"id"`
	OriginalName     string             `json:"original_name"`
	OriginalLanguage string             `json:"original_language"`
	FirstAirDate     string             `json:"first_air_date"`
	NumberOfEpisodes int                `json:"number_of_episodes"`
	PosterPath       string             `json:"poster_path"`
	BackdropPath     string             `json:"backdrop_path"`
	Genres           []apiGenre         `json:"genres"`
	Seasons          []apiSeasonSummary `json:"seasons"`
}

// Series returns one series with its localized text, genres, seasons and logos.
func (c *Client) Series(ctx context.Context, language string, id int64) (domain.Series, error) {
	if id <= 0 {
		return domain.Series{}, domain.ErrNotFound
	}
	base := "/tv/" + strconv.FormatInt(id, 10)
	var raw apiSeries
	if err := c.get(ctx, base, url.Values{"language": {language}}, &raw); err != nil {
		return domain.Series{}, err
	}
	if raw.ID <= 0 {
		return domain.Series{}, c.malformed(base, "id")
	}
	translations, err := c.translations(ctx, base+"/translations")
	if err != nil {
		return domain.Series{}, err
	}
	logos, err := c.logos(ctx, base+"/images", language)
	if err != nil {
		return domain.Series{}, err
	}
	title, overview := domain.Localize(raw.OriginalName, translations, language)

	seasons := make([]domain.SeasonSummary, 0, len(raw.Seasons))
	for _, s := range raw.Seasons {
		seasons = append(seasons, domain.SeasonSummary{
			Number:       s.SeasonNumber,
			EpisodeCount: s.EpisodeCount,
			AirDate:      parseDate(s.AirDate),
			PosterPath:   validPath(s.PosterPath),
		})
	}
	return domain.Series{
		ID:               raw.ID,
		Title:            title,
		EnglishTitle:     domain.EnglishTitle(translations),
		OriginalTitle:    raw.OriginalName,
		OriginalLanguage: raw.OriginalLanguage,
		Overview:         overview,
		Year:             domain.YearOf(parseDate(raw.FirstAirDate)),
		Genres:           toGenres(raw.Genres),
		Seasons:          seasons,
		EpisodeCount:     max(raw.NumberOfEpisodes, 0),
		Images: domain.Images{
			Poster:   validPath(raw.PosterPath),
			Backdrop: validPath(raw.BackdropPath),
			Logos:    logos,
		},
	}, nil
}

type apiEpisode struct {
	ID            int64  `json:"id"`
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"`
	Runtime       int    `json:"runtime"`
	StillPath     string `json:"still_path"`
}

// Season returns the episodes of one season in language. Season 0 holds the specials.
func (c *Client) Season(ctx context.Context, language string, seriesID int64, number int) (domain.Season, error) {
	if seriesID <= 0 || number < 0 {
		return domain.Season{}, domain.ErrNotFound
	}
	endpoint := "/tv/" + strconv.FormatInt(seriesID, 10) + "/season/" + strconv.Itoa(number)
	var raw struct {
		Episodes []apiEpisode `json:"episodes"`
	}
	if err := c.get(ctx, endpoint, url.Values{"language": {language}}, &raw); err != nil {
		return domain.Season{}, err
	}
	episodes := make([]domain.Episode, 0, len(raw.Episodes))
	for _, e := range raw.Episodes {
		if e.EpisodeNumber <= 0 {
			return domain.Season{}, c.malformed(endpoint, "episode_number")
		}
		episodes = append(episodes, domain.Episode{
			ID:           e.ID,
			SeasonNumber: number,
			Number:       e.EpisodeNumber,
			Title:        e.Name,
			Overview:     e.Overview,
			AirDate:      parseDate(e.AirDate),
			Runtime:      max(e.Runtime, 0),
			StillPath:    validPath(e.StillPath),
		})
	}
	return domain.Season{SeriesID: seriesID, Number: number, Episodes: episodes}, nil
}

// ---------- collections ----------

type apiCollectionPart struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	PosterPath    string `json:"poster_path"`
}

type apiCollection struct {
	ID           int64               `json:"id"`
	Name         string              `json:"name"`
	Overview     string              `json:"overview"`
	PosterPath   string              `json:"poster_path"`
	BackdropPath string              `json:"backdrop_path"`
	Parts        []apiCollectionPart `json:"parts"`
}

// Collection returns a collection and its movies in language.
func (c *Client) Collection(ctx context.Context, language string, id int64) (domain.Collection, error) {
	if id <= 0 {
		return domain.Collection{}, domain.ErrNotFound
	}
	endpoint := "/collection/" + strconv.FormatInt(id, 10)
	var raw apiCollection
	if err := c.get(ctx, endpoint, url.Values{"language": {language}}, &raw); err != nil {
		return domain.Collection{}, err
	}
	if raw.ID <= 0 {
		return domain.Collection{}, c.malformed(endpoint, "id")
	}
	parts := make([]domain.CollectionPart, 0, len(raw.Parts))
	for _, p := range raw.Parts {
		if p.ID <= 0 {
			return domain.Collection{}, c.malformed(endpoint, "parts.id")
		}
		parts = append(parts, domain.CollectionPart{
			ID:            p.ID,
			Title:         p.Title,
			OriginalTitle: p.OriginalTitle,
			Year:          domain.YearOf(parseDate(p.ReleaseDate)),
			PosterPath:    validPath(p.PosterPath),
		})
	}
	return domain.Collection{
		ID:           raw.ID,
		Name:         raw.Name,
		Overview:     raw.Overview,
		PosterPath:   validPath(raw.PosterPath),
		BackdropPath: validPath(raw.BackdropPath),
		Parts:        parts,
	}, nil
}

// ---------- genres ----------

// Genres returns the genre list of a kind in language.
func (c *Client) Genres(ctx context.Context, language string, kind domain.MediaKind) ([]domain.Genre, error) {
	var endpoint string
	switch kind {
	case domain.KindMovie:
		endpoint = "/genre/movie/list"
	case domain.KindSeries:
		endpoint = "/genre/tv/list"
	default:
		return nil, fmt.Errorf("tmdb: unknown media kind %q", kind)
	}
	var raw struct {
		Genres []apiGenre `json:"genres"`
	}
	if err := c.get(ctx, endpoint, url.Values{"language": {language}}, &raw); err != nil {
		return nil, err
	}
	for _, g := range raw.Genres {
		if g.ID <= 0 {
			return nil, c.malformed(endpoint, "id")
		}
	}
	return toGenres(raw.Genres), nil
}

// ---------- translations and logos ----------

type apiTranslation struct {
	ISO639  string `json:"iso_639_1"`
	ISO3166 string `json:"iso_3166_1"`
	Data    struct {
		Title    string `json:"title"`
		Name     string `json:"name"`
		Overview string `json:"overview"`
	} `json:"data"`
}

// translations reads a translations endpoint. Movies name their title "title"
// and series "name"; both are accepted.
func (c *Client) translations(ctx context.Context, endpoint string) ([]domain.Translation, error) {
	var raw struct {
		Translations []apiTranslation `json:"translations"`
	}
	if err := c.get(ctx, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]domain.Translation, 0, len(raw.Translations))
	for _, t := range raw.Translations {
		if t.ISO639 == "" {
			continue
		}
		lang := t.ISO639
		if t.ISO3166 != "" {
			lang += "-" + t.ISO3166
		}
		out = append(out, domain.Translation{
			Language: lang,
			Title:    firstNonEmpty(t.Data.Title, t.Data.Name),
			Overview: t.Data.Overview,
		})
	}
	return out, nil
}

// logos reads the logos in the configured language, in English and without text.
func (c *Client) logos(ctx context.Context, endpoint, language string) ([]domain.Logo, error) {
	primary, _, _ := strings.Cut(language, "-")
	langs := []string{primary}
	if primary != "en" {
		langs = append(langs, "en")
	}
	langs = append(langs, "null")
	var raw struct {
		Logos []struct {
			FilePath    string  `json:"file_path"`
			ISO639      string  `json:"iso_639_1"`
			VoteAverage float64 `json:"vote_average"`
			VoteCount   int     `json:"vote_count"`
		} `json:"logos"`
	}
	query := url.Values{"include_image_language": {strings.Join(langs, ",")}}
	if err := c.get(ctx, endpoint, query, &raw); err != nil {
		return nil, err
	}
	var out []domain.Logo
	for _, l := range raw.Logos {
		if path := validPath(l.FilePath); path != "" {
			out = append(out, domain.Logo{Path: path, Language: l.ISO639, VoteAverage: l.VoteAverage, VoteCount: l.VoteCount})
		}
	}
	return out, nil
}

// ---------- helpers ----------

// malformed reports a response that does not have the fields the app needs.
func (c *Client) malformed(endpoint, field string) error {
	return c.upstream(endpoint, "malformed response: "+field)
}

func toGenres(raw []apiGenre) []domain.Genre {
	out := make([]domain.Genre, 0, len(raw))
	for _, g := range raw {
		out = append(out, domain.Genre{ID: g.ID, Name: g.Name})
	}
	return out
}

func parseDate(s string) time.Time {
	t, _ := domain.ParseDate(s)
	return t
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
