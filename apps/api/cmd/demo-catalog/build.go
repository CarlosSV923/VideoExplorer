package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// The demo's languages: the Spanish of the app (RF-61, TMDB_LANGUAGE by
// default, with its own fallbacks) and English. Fixed here so the file does
// not change with the environment.
const (
	spanishLanguage = "es-MX"
	englishLanguage = "en-US"

	catalogSource = "TMDB (https://www.themoviedb.org): structure only; synopses written by the project"
)

// titleKey identifies a title in TMDB's catalogue.
type titleKey struct {
	kind domain.MediaKind
	id   int64
}

// titleInfo is a built title plus TMDB's own overview, kept only to refuse a
// synopsis that copies it (spec RF-71). The overview never reaches the file.
type titleInfo struct {
	title    catalogTitle
	overview localized
}

// build turns the curated selection into the catalog. It reads TMDB through p
// and applies the language policy of RF-61 with the metadata service. Movies
// of the selection bring their whole collection; series bring every season
// that TMDB lists, specials included. Each title needs a synopsis in syn, and
// every synopsis must belong to a title of the catalog.
func build(ctx context.Context, p application.Provider, cur []curatedTitle, syn synopsesFile) (catalog, error) {
	es := application.NewService(p, spanishLanguage)
	en := application.NewService(p, englishLanguage)

	expected := make(map[titleKey]string, len(cur))
	for _, c := range cur {
		expected[titleKey{c.kind, c.id}] = c.original
	}
	movieIDs, seriesIDs, err := selectIDs(ctx, es, cur)
	if err != nil {
		return catalog{}, err
	}

	var titles []catalogTitle
	var errs []error
	for _, id := range movieIDs {
		info, err := movieTitle(ctx, es, en, id, expected[titleKey{domain.KindMovie, id}])
		if err != nil {
			return catalog{}, err
		}
		t, err := attachSynopsis(info, syn.Movies)
		if err != nil {
			errs = append(errs, err)
		}
		titles = append(titles, t)
	}
	for _, id := range seriesIDs {
		info, err := seriesTitle(ctx, p, es, en, id, expected[titleKey{domain.KindSeries, id}])
		if err != nil {
			return catalog{}, err
		}
		t, err := attachSynopsis(info, syn.Series)
		if err != nil {
			errs = append(errs, err)
		}
		titles = append(titles, t)
	}
	errs = append(errs, unusedSynopses(syn.Movies, movieIDs, "movie"), unusedSynopses(syn.Series, seriesIDs, "series"))
	if err := errors.Join(errs...); err != nil {
		return catalog{}, err
	}

	slices.SortFunc(titles, compareTitles)
	c := catalog{Source: catalogSource, Titles: titles}
	if err := validate(c); err != nil {
		return catalog{}, err
	}
	return c, nil
}

// selectIDs returns the ids of the movies and series to build, sorted and
// without repeats. A curated movie adds the other films of its collection.
func selectIDs(ctx context.Context, es *application.Service, cur []curatedTitle) (movies, series []int64, err error) {
	for _, c := range cur {
		switch c.kind {
		case domain.KindSeries:
			series = append(series, c.id)
		case domain.KindMovie:
			movies = append(movies, c.id)
			m, err := es.Movie(ctx, c.id)
			if err != nil {
				return nil, nil, fmt.Errorf("movie %d: %w", c.id, err)
			}
			if m.Collection == nil {
				continue
			}
			col, err := es.Collection(ctx, m.Collection.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("collection %d: %w", m.Collection.ID, err)
			}
			for _, part := range col.Parts {
				movies = append(movies, part.ID)
			}
		default:
			return nil, nil, fmt.Errorf("curated title %d has unknown kind %q", c.id, c.kind)
		}
	}
	return sortedUnique(movies), sortedUnique(series), nil
}

func movieTitle(ctx context.Context, es, en *application.Service, id int64, want string) (titleInfo, error) {
	m, err := es.Movie(ctx, id)
	if err != nil {
		return titleInfo{}, fmt.Errorf("movie %d: %w", id, err)
	}
	if err := checkOriginal("movie", id, m.OriginalTitle, want); err != nil {
		return titleInfo{}, err
	}
	if m.Year == nil {
		return titleInfo{}, fmt.Errorf("movie %d has no release year", id)
	}
	em, err := en.Movie(ctx, id)
	if err != nil {
		return titleInfo{}, fmt.Errorf("movie %d (en): %w", id, err)
	}
	t := catalogTitle{
		TMDBID:           id,
		Type:             typeMovie,
		Year:             *m.Year,
		OriginalTitle:    m.OriginalTitle,
		OriginalLanguage: m.OriginalLanguage,
		Title:            localized{ES: m.Title, EN: em.Title},
		DiscTitle:        domain.DiscTitle(m.OriginalTitle, m.EnglishTitle),
		Runtime:          m.Runtime,
		Genres:           genresOf(m.Genres, em.Genres),
	}
	if m.Collection != nil {
		c, err := collectionOf(ctx, es, en, m.Collection.ID)
		if err != nil {
			return titleInfo{}, err
		}
		t.Collection = &c
	}
	return titleInfo{title: t, overview: localized{ES: m.Overview, EN: em.Overview}}, nil
}

func seriesTitle(ctx context.Context, p application.Provider, es, en *application.Service, id int64, want string) (titleInfo, error) {
	s, err := es.Series(ctx, id)
	if err != nil {
		return titleInfo{}, fmt.Errorf("series %d: %w", id, err)
	}
	if err := checkOriginal("series", id, s.OriginalTitle, want); err != nil {
		return titleInfo{}, err
	}
	if s.Year == nil {
		return titleInfo{}, fmt.Errorf("series %d has no first air year", id)
	}
	enS, err := en.Series(ctx, id)
	if err != nil {
		return titleInfo{}, fmt.Errorf("series %d (en): %w", id, err)
	}
	seasons, err := seasonsOf(ctx, p, es, en, s)
	if err != nil {
		return titleInfo{}, err
	}
	t := catalogTitle{
		TMDBID:           id,
		Type:             typeSeries,
		Year:             *s.Year,
		OriginalTitle:    s.OriginalTitle,
		OriginalLanguage: s.OriginalLanguage,
		Title:            localized{ES: s.Title, EN: enS.Title},
		DiscTitle:        domain.DiscTitle(s.OriginalTitle, s.EnglishTitle),
		Genres:           genresOf(s.Genres, enS.Genres),
		Seasons:          seasons,
	}
	return titleInfo{title: t, overview: localized{ES: s.Overview, EN: enS.Overview}}, nil
}

// seasonsOf reads every season TMDB lists for the series, in number order. The
// chapter titles come in Spanish and English, and the original-language title
// of each chapter is read only to pick the chapter's disc title (spec §5).
func seasonsOf(ctx context.Context, p application.Provider, es, en *application.Service, s domain.Series) ([]catalogSeason, error) {
	numbers := make([]int, 0, len(s.Seasons))
	for _, summary := range s.Seasons {
		numbers = append(numbers, summary.Number)
	}
	slices.Sort(numbers)
	numbers = slices.Compact(numbers)

	out := make([]catalogSeason, 0, len(numbers))
	for _, n := range numbers {
		esSeason, err := es.Season(ctx, s.ID, n)
		if err != nil {
			return nil, fmt.Errorf("series %d season %d: %w", s.ID, n, err)
		}
		enSeason, err := en.Season(ctx, s.ID, n)
		if err != nil {
			return nil, fmt.Errorf("series %d season %d (en): %w", s.ID, n, err)
		}
		originals, err := originalChapterTitles(ctx, p, s.OriginalLanguage, s.ID, n)
		if err != nil {
			return nil, err
		}
		enTitles := make(map[int]string, len(enSeason.Episodes))
		for _, e := range enSeason.Episodes {
			enTitles[e.Number] = e.Title
		}
		episodes := slices.Clone(esSeason.Episodes)
		slices.SortFunc(episodes, func(a, b domain.Episode) int { return a.Number - b.Number })
		chapters := make([]catalogEpisode, 0, len(episodes))
		for _, e := range episodes {
			chapters = append(chapters, catalogEpisode{
				Number:    e.Number,
				Title:     localized{ES: e.Title, EN: enTitles[e.Number]},
				DiscTitle: domain.ChapterTitle([]domain.ChapterNames{{Original: originals[e.Number], English: enTitles[e.Number]}}),
				AirDate:   airDate(e.AirDate),
			})
		}
		out = append(out, catalogSeason{Number: n, Episodes: chapters})
	}
	return out, nil
}

// originalChapterTitles returns the chapter titles in the series' original
// language, by episode number. Empty when TMDB has no such season.
func originalChapterTitles(ctx context.Context, p application.Provider, language string, seriesID int64, number int) (map[int]string, error) {
	if language == "" {
		return nil, nil
	}
	season, err := p.Season(ctx, language, seriesID, number)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("series %d season %d (%s): %w", seriesID, number, language, err)
	}
	titles := make(map[int]string, len(season.Episodes))
	for _, e := range season.Episodes {
		titles[e.Number] = e.Title
	}
	return titles, nil
}

func collectionOf(ctx context.Context, es, en *application.Service, id int64) (catalogCollection, error) {
	ec, err := es.Collection(ctx, id)
	if err != nil {
		return catalogCollection{}, fmt.Errorf("collection %d: %w", id, err)
	}
	nc, err := en.Collection(ctx, id)
	if err != nil {
		return catalogCollection{}, fmt.Errorf("collection %d (en): %w", id, err)
	}
	return catalogCollection{ID: id, Name: localized{ES: ec.Name, EN: nc.Name}}, nil
}

// genresOf pairs each genre of the Spanish list with its English name.
func genresOf(es, en []domain.Genre) []catalogGenre {
	names := make(map[int64]string, len(en))
	for _, g := range en {
		names[g.ID] = g.Name
	}
	out := make([]catalogGenre, 0, len(es))
	for _, g := range es {
		out = append(out, catalogGenre{ID: g.ID, Name: localized{ES: g.Name, EN: names[g.ID]}})
	}
	return out
}

// attachSynopsis sets the project's synopsis of a title. It fails when the
// synopsis is missing or copies TMDB's overview.
func attachSynopsis(info titleInfo, syn map[int64]localized) (catalogTitle, error) {
	t := info.title
	s, ok := syn[t.TMDBID]
	if !ok {
		return t, fmt.Errorf("missing synopsis for %s %d (%s)", t.Type, t.TMDBID, t.OriginalTitle)
	}
	if sameText(s.ES, info.overview.ES) || sameText(s.EN, info.overview.EN) {
		return t, fmt.Errorf("synopsis of %s %d copies the TMDB overview", t.Type, t.TMDBID)
	}
	t.Synopsis = s
	return t, nil
}

// unusedSynopses reports synopses whose id is not in the selection.
func unusedSynopses(syn map[int64]localized, ids []int64, kind string) error {
	var errs []error
	for id := range syn {
		if !slices.Contains(ids, id) {
			errs = append(errs, fmt.Errorf("synopsis of %s %d matches no title of the catalog", kind, id))
		}
	}
	return errors.Join(errs...)
}

// checkOriginal fails when TMDB's original title is not the one the selection expects.
func checkOriginal(kind string, id int64, got, want string) error {
	if want != "" && got != want {
		return fmt.Errorf("%s %d is %q in TMDB, not %q", kind, id, got, want)
	}
	return nil
}

// compareTitles orders movies before series, then by year and id.
func compareTitles(a, b catalogTitle) int {
	if a.Type != b.Type {
		return strings.Compare(a.Type, b.Type)
	}
	if a.Year != b.Year {
		return a.Year - b.Year
	}
	switch {
	case a.TMDBID < b.TMDBID:
		return -1
	case a.TMDBID > b.TMDBID:
		return 1
	}
	return 0
}

func sortedUnique(ids []int64) []int64 {
	slices.Sort(ids)
	return slices.Compact(ids)
}

// sameText compares two texts ignoring case and spacing. An empty text is never the same.
func sameText(a, b string) bool {
	a = strings.Join(strings.Fields(a), " ")
	b = strings.Join(strings.Fields(b), " ")
	return a != "" && strings.EqualFold(a, b)
}

func airDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateOnly)
}
