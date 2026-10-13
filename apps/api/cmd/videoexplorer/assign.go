package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
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
		Section: catalogshttp.NewSectionHandler(catalog.Section, arrivalsOf{ingest: ingest}, log),
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
			out = append(out, catalogapplication.TMDBTitle{ID: h.ID, Title: h.Title, Year: intOf(h.Year), PosterFile: posterFile(h.PosterPath)})
		}
		return out, nil
	}
	hits, err := t.svc.SearchMovies(ctx, query, yearArg)
	if err != nil {
		return nil, tmdbErr(err)
	}
	out := make([]catalogapplication.TMDBTitle, 0, len(hits))
	for _, h := range hits {
		out = append(out, catalogapplication.TMDBTitle{ID: h.ID, Title: h.Title, Year: intOf(h.Year), PosterFile: posterFile(h.PosterPath)})
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
			ID: s.ID, Title: s.Title, DiscTitle: metadatadomain.DiscTitle(s.OriginalTitle, s.EnglishTitle),
			Year: intOf(s.Year), PosterFile: posterFile(s.Images.Poster),
		}, nil
	}
	m, err := t.svc.Movie(ctx, id)
	if err != nil {
		return catalogapplication.TMDBTitle{}, tmdbErr(err)
	}
	return catalogapplication.TMDBTitle{
		ID: m.ID, Title: m.Title, DiscTitle: metadatadomain.DiscTitle(m.OriginalTitle, m.EnglishTitle),
		Year: intOf(m.Year), PosterFile: posterFile(m.Images.Poster),
	}, nil
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
