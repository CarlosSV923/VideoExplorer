package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogdomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	ingestiondomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/ffprobe"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	metadataapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	metadatadomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// assignWiring is the assignment of No asignados (RF-20 to RF-26) as the composition root builds it.
type assignWiring struct {
	Handler *catalogshttp.Handler
	// Section is No asignados as the disk holds it, and the changes of its entries (RF-27).
	Section *catalogshttp.SectionHandler
}

// newAssign wires the assignment over the catalog's engine and registry. TMDB is the metadata
// service, the names are read by the ingestion's analysis, the streams of the videos by ffprobe and
// the partial hashes by the ingestion service (which refuses links, RNF-02). Each is adapted here, so
// that the catalog keeps the types of the other contexts out.
func newAssign(cfg config.Config, conn *sql.DB, catalog catalogWiring, meta metadataWiring, ingest *ingestionapplication.Service, log *slog.Logger) assignWiring {
	files := libraryfs.New(cfg.LibraryPath)
	library := catalogsqlite.NewLibraryRepository(conn)
	prober := ingestionapplication.NewCachedProber(ffprobe.New(""), fileid.Identifier{}, ingestionsqlite.NewMediaProbeRepository(conn), log)
	svc := catalogapplication.NewAssignments(catalogapplication.AssignDeps{
		Unassigned:   catalogsqlite.NewUnassignedRepository(conn),
		Library:      library,
		Files:        files,
		Engine:       catalog.Engine,
		Finder:       catalogapplication.NewDuplicateFinder(library, files, hasherOf{ingest: ingest}, cfg.VideoExtensions),
		Media:        mediaOf{prober: prober, log: log},
		TMDB:         tmdbOf{svc: meta.Service},
		Names:        namesOf{},
		Roots:        catalogapplication.Roots{Movies: cfg.MoviesDir, Series: cfg.SeriesDir},
		VideoExts:    cfg.VideoExtensions,
		SubtitleExts: cfg.SubtitleExtensions,
		Now:          time.Now,
		Log:          log,
	})
	return assignWiring{
		Handler: catalogshttp.NewHandler(svc, log),
		Section: catalogshttp.NewSectionHandler(catalog.Section, arrivalsOf{ingest: ingest}),
	}
}

// hasherOf gives the catalog the partial hash of a video under the library (RF-24).
type hasherOf struct {
	ingest *ingestionapplication.Service
}

// PartialHash implements catalogapplication.PartialHasher.
func (h hasherOf) PartialHash(ctx context.Context, path string) (string, error) {
	return h.ingest.PartialHashOf(ctx, path)
}

// mediaOf gives the catalog what a version label reads from a video (RF-24). A video that ffprobe
// cannot read has no media, so its label stays incomplete: nothing is invented (RF-24).
type mediaOf struct {
	prober *ingestionapplication.CachedProber
	log    *slog.Logger
}

// Media implements catalogapplication.MediaReader.
func (m mediaOf) Media(ctx context.Context, path string) (catalogdomain.VersionMedia, error) {
	info, err := m.prober.Probe(ctx, path)
	if err != nil {
		m.log.WarnContext(ctx, "ffprobe could not read a version; its label stays incomplete", "path", path, "error", err)
		return catalogdomain.VersionMedia{}, nil
	}
	out := catalogdomain.VersionMedia{Width: info.Video.Width, Height: info.Video.Height, Codec: info.Video.Codec}
	for _, a := range info.Audio {
		out.Audio = append(out.Audio, catalogdomain.AudioMedia{Language: a.Language, Title: a.Title})
	}
	return out, nil
}

// namesOf reads what a file name says with the ingestion's analysis (RF-20, RF-23).
type namesOf struct{}

// Read implements catalogapplication.NameReader.
func (namesOf) Read(rel string, subtitle bool) catalogapplication.NameReading {
	kind := ingestiondomain.KindVideo
	if subtitle {
		kind = ingestiondomain.KindSubtitle
	}
	a := ingestiondomain.AnalyzeFile(rel, kind)
	out := catalogapplication.NameReading{
		Title: a.Title, Year: a.Year, Season: a.Season, Chapter: a.Chapter, Last: a.LastChapter,
		Part: a.Part, Language: a.Language, Forced: a.Forced, SDH: a.SDH,
	}
	switch a.Type {
	case ingestiondomain.TypeMovie:
		out.Kind = catalogdomain.KindMovie
	case ingestiondomain.TypeEpisode:
		out.Kind = catalogdomain.KindSeries
	}
	return out
}

// tmdbOf gives the catalog the titles of TMDB, named as spec §5 names them (RF-21, RF-22, §6).
type tmdbOf struct {
	svc *metadataapplication.Service
}

// Configured implements catalogapplication.TMDB.
func (t tmdbOf) Configured() bool { return t.svc.Configured() }

// SearchTitles implements catalogapplication.TMDB.
func (t tmdbOf) SearchTitles(ctx context.Context, kind catalogdomain.TitleKind, query string, year int) ([]catalogapplication.TMDBTitle, error) {
	var yearArg *int
	if year > 0 {
		yearArg = &year
	}
	if kind == catalogdomain.KindSeries {
		hits, err := t.svc.SearchSeries(ctx, query, yearArg)
		if err != nil {
			return nil, tmdbErr(err)
		}
		out := make([]catalogapplication.TMDBTitle, 0, len(hits))
		for _, h := range hits {
			out = append(out, catalogapplication.TMDBTitle{ID: h.ID, Title: h.Title, OriginalTitle: h.OriginalTitle, Year: intOf(h.Year), PosterFile: posterFile(h.PosterPath)})
		}
		return out, nil
	}
	hits, err := t.svc.SearchMovies(ctx, query, yearArg)
	if err != nil {
		return nil, tmdbErr(err)
	}
	out := make([]catalogapplication.TMDBTitle, 0, len(hits))
	for _, h := range hits {
		out = append(out, catalogapplication.TMDBTitle{ID: h.ID, Title: h.Title, OriginalTitle: h.OriginalTitle, Year: intOf(h.Year), PosterFile: posterFile(h.PosterPath)})
	}
	return out, nil
}

// Title implements catalogapplication.TMDB: the disc title <T> (spec §5) and the year of a movie or a series.
func (t tmdbOf) Title(ctx context.Context, kind catalogdomain.TitleKind, id int64) (catalogapplication.TMDBTitle, error) {
	if kind == catalogdomain.KindSeries {
		s, err := t.svc.Series(ctx, id)
		if err != nil {
			return catalogapplication.TMDBTitle{}, tmdbErr(err)
		}
		return catalogapplication.TMDBTitle{
			ID: s.ID, Title: s.Title, OriginalTitle: s.OriginalTitle, DiscTitle: metadatadomain.DiscTitle(s.OriginalTitle, s.EnglishTitle),
			Year: intOf(s.Year), PosterFile: posterFile(s.Images.Poster),
		}, nil
	}
	m, err := t.svc.Movie(ctx, id)
	if err != nil {
		return catalogapplication.TMDBTitle{}, tmdbErr(err)
	}
	return catalogapplication.TMDBTitle{
		ID: m.ID, Title: m.Title, OriginalTitle: m.OriginalTitle, DiscTitle: metadatadomain.DiscTitle(m.OriginalTitle, m.EnglishTitle),
		Year: intOf(m.Year), PosterFile: posterFile(m.Images.Poster),
	}, nil
}

// Details implements catalogapplication.LibraryMetadata for the title page (RF-43a). The service already
// supplies cached, localized metadata; this adapter only translates the metadata context's types into the
// catalog port and chooses the configured-language logo.
func (t tmdbOf) Details(ctx context.Context, kind catalogdomain.TitleKind, id int64) (catalogapplication.TitleMetadata, error) {
	return t.details(ctx, kind, id, false)
}

// StoredSummary reads only the metadata response cache. The library list must
// remain available without turning stale entries into network calls (RF-42a).
func (t tmdbOf) StoredSummary(ctx context.Context, kind catalogdomain.TitleKind, id int64) (catalogapplication.TitleSummaryMetadata, bool, error) {
	if kind == catalogdomain.KindSeries {
		series, found, err := t.svc.StoredSeries(ctx, id)
		if err != nil || !found {
			return catalogapplication.TitleSummaryMetadata{}, found, err
		}
		return catalogapplication.TitleSummaryMetadata{
			OriginalTitle: series.OriginalTitle,
			Poster:        posterFile(series.Images.Poster),
			Backdrop:      posterFile(series.Images.Backdrop),
			Genres:        genreNames(series.Genres),
		}, true, nil
	}
	movie, found, err := t.svc.StoredMovie(ctx, id)
	if err != nil || !found {
		return catalogapplication.TitleSummaryMetadata{}, found, err
	}
	var collection *catalogapplication.CollectionRef
	if movie.Collection != nil {
		collection = &catalogapplication.CollectionRef{ID: movie.Collection.ID, Name: movie.Collection.Name}
	}
	return catalogapplication.TitleSummaryMetadata{
		OriginalTitle: movie.OriginalTitle,
		Poster:        posterFile(movie.Images.Poster),
		Backdrop:      posterFile(movie.Images.Backdrop),
		Genres:        genreNames(movie.Genres),
		Collection:    collection,
	}, true, nil
}

// Collection returns the regular cache-aware collection metadata for the
// collection screen (RF-40, RF-42a).
func (t tmdbOf) Collection(ctx context.Context, id int64) (catalogapplication.CollectionMetadata, error) {
	collection, err := t.svc.Collection(ctx, id)
	if err != nil {
		return catalogapplication.CollectionMetadata{}, tmdbErr(err)
	}
	return *collectionMetadata(collection), nil
}

// Refresh implements catalogapplication.LibraryMetadataRefresher (RF-44). All
// cache writes remain staged until the complete metadata graph has been read;
// a TMDB error therefore cannot replace only part of the previous answer.
func (t tmdbOf) Refresh(ctx context.Context, kind catalogdomain.TitleKind, id int64) (catalogapplication.TitleMetadata, error) {
	refreshCtx, tx := metadataapplication.BeginRefresh(metadataapplication.ForceRefresh(ctx))
	out, err := t.details(refreshCtx, kind, id, true)
	if err != nil {
		return catalogapplication.TitleMetadata{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return catalogapplication.TitleMetadata{}, fmt.Errorf("%w: metadata cache commit: %w", catalogapplication.ErrTMDBUnavailable, err)
	}
	return out, nil
}

func (t tmdbOf) details(ctx context.Context, kind catalogdomain.TitleKind, id int64, strictCollection bool) (catalogapplication.TitleMetadata, error) {
	if kind == catalogdomain.KindSeries {
		series, err := t.svc.Series(ctx, id)
		if err != nil {
			return catalogapplication.TitleMetadata{}, tmdbErr(err)
		}
		out := catalogapplication.TitleMetadata{
			Title: series.Title, OriginalTitle: series.OriginalTitle, Year: intOf(series.Year), Overview: series.Overview,
			Genres: genreNames(series.Genres), Poster: posterFile(series.Images.Poster), Backdrop: posterFile(series.Images.Backdrop),
			Logo:    chosenLogo(series.Images.Logos, t.svc.Language()),
			Seasons: make([]catalogapplication.SeasonMetadata, 0, len(series.Seasons)),
		}
		seasons := make([]catalogapplication.SeasonMetadata, len(series.Seasons))
		var wg sync.WaitGroup
		var firstErr error
		var errMu sync.Mutex
		seasonCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		for i, summary := range series.Seasons {
			wg.Add(1)
			go func() {
				defer wg.Done()
				season, err := t.svc.Season(seasonCtx, id, summary.Number)
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					errMu.Unlock()
					return
				}
				seasons[i] = seasonMetadata(season)
			}()
		}
		wg.Wait()
		if firstErr != nil {
			return catalogapplication.TitleMetadata{}, tmdbErr(firstErr)
		}
		out.Seasons = seasons
		return out, nil
	}
	movie, err := t.svc.Movie(ctx, id)
	if err != nil {
		return catalogapplication.TitleMetadata{}, tmdbErr(err)
	}
	out := catalogapplication.TitleMetadata{
		Title: movie.Title, OriginalTitle: movie.OriginalTitle, Year: intOf(movie.Year), Runtime: movie.Runtime, Overview: movie.Overview,
		Genres: genreNames(movie.Genres), Poster: posterFile(movie.Images.Poster), Backdrop: posterFile(movie.Images.Backdrop),
		Logo: chosenLogo(movie.Images.Logos, t.svc.Language()),
	}
	if movie.Collection != nil {
		collection, err := t.svc.Collection(ctx, movie.Collection.ID)
		if err != nil {
			if strictCollection {
				return catalogapplication.TitleMetadata{}, tmdbErr(err)
			}
		} else {
			out.Collection = collectionMetadata(collection)
		}
	}
	return out, nil
}

func genreNames(genres []metadatadomain.Genre) []string {
	out := make([]string, 0, len(genres))
	for _, genre := range genres {
		out = append(out, genre.Name)
	}
	return out
}

func chosenLogo(logos []metadatadomain.Logo, language string) string {
	logo, ok := metadatadomain.PickLogo(logos, language)
	if !ok {
		return ""
	}
	return posterFile(logo.Path)
}

func seasonMetadata(season metadatadomain.Season) catalogapplication.SeasonMetadata {
	out := catalogapplication.SeasonMetadata{Number: season.Number, Episodes: make([]catalogapplication.EpisodeMetadata, 0, len(season.Episodes))}
	for _, episode := range season.Episodes {
		out.Episodes = append(out.Episodes, catalogapplication.EpisodeMetadata{ID: episode.ID, Number: episode.Number, Title: episode.Title, Overview: episode.Overview, Still: posterFile(episode.StillPath), AirDate: episode.AirDate})
	}
	return out
}

func collectionMetadata(collection metadatadomain.Collection) *catalogapplication.CollectionMetadata {
	out := &catalogapplication.CollectionMetadata{ID: collection.ID, Name: collection.Name, Overview: collection.Overview, Poster: posterFile(collection.PosterPath), Backdrop: posterFile(collection.BackdropPath), Movies: make([]catalogapplication.CollectionMovieMetadata, 0, len(collection.Parts))}
	for _, movie := range collection.Parts {
		out.Movies = append(out.Movies, catalogapplication.CollectionMovieMetadata{ID: movie.ID, Title: movie.Title, OriginalTitle: movie.OriginalTitle, Year: intOf(movie.Year), Poster: posterFile(movie.PosterPath)})
	}
	return out
}

// FindIMDb implements catalogapplication.ImportTMDB: the titles of one kind that TMDB links to an IMDb id (RF-29).
func (t tmdbOf) FindIMDb(ctx context.Context, kind catalogdomain.TitleKind, imdbID string) ([]catalogapplication.TMDBTitle, error) {
	found, err := t.svc.FindByIMDb(ctx, imdbID)
	if err != nil {
		return nil, tmdbErr(err)
	}
	var out []catalogapplication.TMDBTitle
	if kind == catalogdomain.KindSeries {
		for _, h := range found.Series {
			out = append(out, catalogapplication.TMDBTitle{ID: h.ID, Title: h.Title, OriginalTitle: h.OriginalTitle, Year: intOf(h.Year), PosterFile: posterFile(h.PosterPath)})
		}
		return out, nil
	}
	for _, h := range found.Movies {
		out = append(out, catalogapplication.TMDBTitle{ID: h.ID, Title: h.Title, OriginalTitle: h.OriginalTitle, Year: intOf(h.Year), PosterFile: posterFile(h.PosterPath)})
	}
	return out, nil
}

// Chapters implements catalogapplication.TMDB: the chapter title of spec §5 for the chapters of a file,
// and the chapters that TMDB does not have (RF-22).
func (t tmdbOf) Chapters(ctx context.Context, seriesID int64, season, first, last int) (catalogapplication.TMDBChapters, error) {
	s, err := t.svc.Season(ctx, seriesID, season)
	if err != nil {
		return catalogapplication.TMDBChapters{}, tmdbErr(err)
	}
	byNumber := map[int]metadatadomain.Episode{}
	for _, e := range s.Episodes {
		byNumber[e.Number] = e
	}
	var names []metadatadomain.ChapterNames
	var missing []int
	for n := first; n <= last; n++ {
		e, ok := byNumber[n]
		if !ok {
			missing = append(missing, n)
			continue
		}
		names = append(names, metadatadomain.ChapterNames{Original: e.OriginalTitle, English: e.EnglishTitle})
	}
	return catalogapplication.TMDBChapters{Title: metadatadomain.ChapterTitle(names), Missing: missing}, nil
}

// tmdbErr maps the metadata context's errors to the ones the catalog knows (spec §6).
func tmdbErr(err error) error {
	switch {
	case errors.Is(err, metadatadomain.ErrNotConfigured):
		return catalogapplication.ErrTMDBNotConfigured
	case errors.Is(err, metadatadomain.ErrNotFound):
		return catalogapplication.ErrTMDBNotFound
	default:
		return fmt.Errorf("%w: %w", catalogapplication.ErrTMDBUnavailable, err)
	}
}

// posterPattern is a TMDB image file name that /api/images serves (RNF-10).
var posterPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}\.(jpe?g|png)$`)

// posterFile returns the file name of a TMDB image path, as /api/images/poster/{file} takes it; "" when
// the path is absent or is not a JPEG or PNG name.
func posterFile(path string) string {
	name := strings.TrimPrefix(path, "/")
	if !posterPattern.MatchString(name) {
		return ""
	}
	return name
}

// intOf returns the value of an optional year, or 0 when it is unknown.
func intOf(year *int) int {
	if year == nil {
		return 0
	}
	return *year
}
