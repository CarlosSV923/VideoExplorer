package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// ErrTitleNotFound means there is no title of the library with that id (or it is only in the trash).
var ErrTitleNotFound = errors.New("the title is not in the library")

// TitleSummary is a title of the library as the list shows it (RF-40, RF-43).
type TitleSummary struct {
	ID    int64
	Kind  domain.TitleKind
	Title string
	Year  int
	// TMDBID is 0 for a free title.
	TMDBID int64
	IDTag  bool
	// MissingEpisodes is the last successfully calculated number of emitted episodes missing from normal
	// seasons. It is nil until the first calculation (RF-40, RF-43a).
	MissingEpisodes *int
}

// TitleDetail is a title with its videos, as the page of the title shows them (RF-43).
type TitleDetail struct {
	TitleSummary
	DiscTitle string
	// Poster is the file name of the poster, "" when TMDB does not give one (spec §6).
	Poster string
	// TMDB fields are empty/zero for free titles or when metadata is unavailable (RF-43a, §6).
	OriginalTitle string
	Overview      string
	Runtime       int
	Genres        []string
	Backdrop      string
	Logo          string
	Collection    *CollectionView
	Seasons       []SeasonView
	Videos        []VideoView
}

// CollectionView is the collection of a movie, with the library state of every part (RF-43a).
type CollectionView struct {
	ID       int64
	Name     string
	Overview string
	Poster   string
	Backdrop string
	Movies   []CollectionMovieView
}

// CollectionMovieView is one movie in a collection. LibraryTitleID is zero when it is absent from the library.
type CollectionMovieView struct {
	ID             int64
	Title          string
	OriginalTitle  string
	Year           int
	Poster         string
	LibraryTitleID int64
}

// SeasonView is a TMDB season, including season 0 (specials) (RF-43a).
type SeasonView struct {
	Number   int
	Episodes []EpisodeView
}

// EpisodeView is a TMDB episode and its library files. A multi-episode file appears in every covered episode.
type EpisodeView struct {
	ID       int64
	Number   int
	Title    string
	Overview string
	Still    string
	AirDate  time.Time
	Status   EpisodeStatus
	Files    []VideoView
}

// EpisodeStatus is the state of a TMDB episode in the library (RF-43a).
type EpisodeStatus string

const (
	// EpisodePresent means a live library file covers the TMDB episode.
	EpisodePresent EpisodeStatus = "present"
	// EpisodeMissing means the episode's air date has passed and no live file covers it.
	EpisodeMissing EpisodeStatus = "missing"
	// EpisodeUpcoming means the episode has a future or unknown air date.
	EpisodeUpcoming EpisodeStatus = "upcoming"
)

// LibraryMetadata is the optional enriched metadata port used by the title page. It is deliberately separate
// from TMDB, whose smaller port is also used by Asignar and importación.
type LibraryMetadata interface {
	Details(ctx context.Context, kind domain.TitleKind, id int64) (TitleMetadata, error)
}

// TitleMetadata is the catalog-shaped answer of the metadata context for RF-43a.
type TitleMetadata struct {
	Title         string
	OriginalTitle string
	Year          int
	Runtime       int
	Overview      string
	Genres        []string
	Poster        string
	Backdrop      string
	Logo          string
	Collection    *CollectionMetadata
	Seasons       []SeasonMetadata
}

// CollectionMetadata is a TMDB movie collection returned through the metadata port.
type CollectionMetadata struct {
	ID       int64
	Name     string
	Overview string
	Poster   string
	Backdrop string
	Movies   []CollectionMovieMetadata
}

// CollectionMovieMetadata is one movie in a TMDB collection.
type CollectionMovieMetadata struct {
	ID            int64
	Title         string
	OriginalTitle string
	Year          int
	Poster        string
}

// SeasonMetadata is a TMDB season returned through the metadata port.
type SeasonMetadata struct {
	Number   int
	Episodes []EpisodeMetadata
}

// EpisodeMetadata is a TMDB episode returned through the metadata port.
type EpisodeMetadata struct {
	ID       int64
	Number   int
	Title    string
	Overview string
	Still    string
	AirDate  time.Time
}

// VideoView is a video of a title and the subtitles that go with it.
type VideoView struct {
	ID   int64
	Rel  string
	Size int64
	// Place is the part of a movie (0 for the whole movie) or the chapters of a series.
	Place   domain.Place
	Version string
	// ChapterTitle is the title of a chapter of a free series, read from its name; "" otherwise.
	ChapterTitle string
	Subtitles    []SubtitleView
}

// SubtitleView is a subtitle that goes with a video (RF-23).
type SubtitleView struct {
	ID       int64
	Rel      string
	Language string
	Forced   bool
	SDH      bool
}

// LibraryViews answers what the title pages show: the titles of the library and one of them with its
// files (RF-40, RF-43). It changes nothing.
type LibraryViews struct {
	repo LibraryRepository
	tmdb TMDB
	log  *slog.Logger
	now  func() time.Time
}

// NewLibraryViews builds the reads over the registry and TMDB. log may be nil.
func NewLibraryViews(repo LibraryRepository, tmdb TMDB, log *slog.Logger, clocks ...func() time.Time) *LibraryViews {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	return &LibraryViews{repo: repo, tmdb: tmdb, log: log, now: now}
}

// Titles returns the titles of the library: those with at least one file out of the trash (RF-30), by id.
func (v *LibraryViews) Titles(ctx context.Context) ([]TitleSummary, error) {
	live, _, err := classifyTitles(ctx, v.repo)
	if err != nil {
		return nil, err
	}
	out := make([]TitleSummary, 0, len(live))
	for _, t := range live {
		summary := summaryOf(t)
		if t.Kind == domain.KindSeries {
			missing, err := v.storedMissingEpisodes(ctx, t.ID)
			if err != nil {
				return nil, err
			}
			summary.MissingEpisodes = missing
		}
		out = append(out, summary)
	}
	return out, nil
}

// Title returns a title of the library with its files that are out of the trash (RF-43). The poster comes
// from TMDB through its cache when TMDB is configured; a failure there leaves it out and is logged.
func (v *LibraryViews) Title(ctx context.Context, id int64) (TitleDetail, error) {
	live, _, err := classifyTitles(ctx, v.repo)
	if err != nil {
		return TitleDetail{}, err
	}
	var title domain.LibraryTitle
	found := false
	for _, t := range live {
		if t.ID == id {
			title, found = t, true
			break
		}
	}
	if !found {
		return TitleDetail{}, ErrTitleNotFound
	}
	files, err := v.repo.Files(ctx, id)
	if err != nil {
		return TitleDetail{}, err
	}
	out := TitleDetail{TitleSummary: summaryOf(title), DiscTitle: title.DiscTitle}
	out.MissingEpisodes = v.readMissingEpisodes(ctx, title.ID)
	out.Videos = videoViews(title, files)
	if title.TMDBID > 0 && v.tmdb != nil && v.tmdb.Configured() {
		if rich, ok := v.tmdb.(LibraryMetadata); ok {
			metadata, err := rich.Details(ctx, title.Kind, title.TMDBID)
			if err != nil {
				v.log.WarnContext(ctx, "metadata of a title not read from TMDB", "title", id, "error", err)
			} else {
				v.applyMetadata(ctx, &out, metadata, title, v.now())
			}
		} else {
			t, err := v.tmdb.Title(ctx, title.Kind, title.TMDBID)
			if err != nil {
				v.log.WarnContext(ctx, "poster of a title not read from TMDB", "title", id, "error", err)
			} else {
				out.Poster = t.PosterFile
			}
		}
	}
	return out, nil
}

// summaryOf returns the list's view of a title.
func summaryOf(t domain.LibraryTitle) TitleSummary {
	return TitleSummary{ID: t.ID, Kind: t.Kind, Title: t.Title, Year: t.Year, TMDBID: t.TMDBID, IDTag: t.IDTag}
}

func (v *LibraryViews) storedMissingEpisodes(ctx context.Context, titleID int64) (*int, error) {
	store, ok := v.repo.(MissingEpisodesStore)
	if !ok {
		return nil, nil
	}
	missing, err := store.MissingEpisodes(ctx, titleID)
	if err != nil {
		return nil, err
	}
	return missing, nil
}

func (v *LibraryViews) readMissingEpisodes(ctx context.Context, titleID int64) *int {
	missing, err := v.storedMissingEpisodes(ctx, titleID)
	if err != nil {
		v.log.WarnContext(ctx, "stored missing episode count not read", "title", titleID, "error", err)
	}
	return missing
}

func (v *LibraryViews) applyMetadata(ctx context.Context, out *TitleDetail, metadata TitleMetadata, title domain.LibraryTitle, now time.Time) {
	out.OriginalTitle = metadata.OriginalTitle
	out.Overview = metadata.Overview
	out.Runtime = metadata.Runtime
	out.Genres = append([]string(nil), metadata.Genres...)
	out.Poster = metadata.Poster
	out.Backdrop = metadata.Backdrop
	out.Logo = metadata.Logo
	if metadata.Title != "" {
		out.Title = metadata.Title
	}
	if metadata.Year != 0 {
		out.Year = metadata.Year
	}
	out.Collection = collectionView(metadata.Collection, v.collectionLibrary(ctx), title)
	if title.Kind == domain.KindSeries {
		var missing int
		out.Seasons, missing = episodeViews(metadata.Seasons, out.Videos, now)
		out.MissingEpisodes = &missing
		if store, ok := v.repo.(MissingEpisodesStore); ok {
			if err := store.SetMissingEpisodes(ctx, title.ID, missing); err != nil {
				v.log.WarnContext(ctx, "missing episode count not saved", "title", title.ID, "error", err)
			}
		}
	}
}

// RefreshMissingEpisodes recalculates the persisted count for every live linked series. It is intended for
// the library scan's background work (RF-28, RF-43a); failures for one title leave its previous value intact.
func (v *LibraryViews) RefreshMissingEpisodes(ctx context.Context) error {
	store, ok := v.repo.(MissingEpisodesStore)
	if !ok || v.tmdb == nil || !v.tmdb.Configured() {
		return nil
	}
	rich, ok := v.tmdb.(LibraryMetadata)
	if !ok {
		return nil
	}
	live, _, err := classifyTitles(ctx, v.repo)
	if err != nil {
		return err
	}
	for _, title := range live {
		if title.Kind != domain.KindSeries || title.TMDBID == 0 {
			continue
		}
		metadata, err := rich.Details(ctx, title.Kind, title.TMDBID)
		if err != nil {
			v.log.WarnContext(ctx, "missing episode count not refreshed from TMDB", "title", title.ID, "error", err)
			continue
		}
		files, err := v.repo.Files(ctx, title.ID)
		if err != nil {
			v.log.WarnContext(ctx, "files not read for missing episode count", "title", title.ID, "error", err)
			continue
		}
		_, missing := episodeViews(metadata.Seasons, videoViews(title, files), v.now())
		if err := store.SetMissingEpisodes(ctx, title.ID, missing); err != nil {
			v.log.WarnContext(ctx, "missing episode count not saved", "title", title.ID, "error", err)
		}
	}
	return nil
}

func (v *LibraryViews) collectionLibrary(ctx context.Context) map[int64]int64 {
	titles, err := v.repo.Titles(ctx)
	if err != nil {
		v.log.WarnContext(ctx, "collection library state not read", "error", err)
		return nil
	}
	linked := make(map[int64]int64)
	for _, candidate := range titles {
		if candidate.Kind != domain.KindMovie || candidate.TMDBID == 0 {
			continue
		}
		files, err := v.repo.Files(ctx, candidate.ID)
		if err == nil && anyLive(files) {
			linked[candidate.TMDBID] = candidate.ID
		}
	}
	return linked
}

func collectionView(metadata *CollectionMetadata, linked map[int64]int64, title domain.LibraryTitle) *CollectionView {
	if metadata == nil {
		return nil
	}
	if linked == nil {
		linked = make(map[int64]int64)
	}
	if title.TMDBID > 0 {
		linked[title.TMDBID] = title.ID
	}
	return &CollectionView{
		ID: metadata.ID, Name: metadata.Name, Overview: metadata.Overview, Poster: metadata.Poster, Backdrop: metadata.Backdrop,
		Movies: collectionMovies(metadata.Movies, linked),
	}
}

func collectionMovies(metadata []CollectionMovieMetadata, linked map[int64]int64) []CollectionMovieView {
	out := make([]CollectionMovieView, 0, len(metadata))
	for _, movie := range metadata {
		out = append(out, CollectionMovieView{ID: movie.ID, Title: movie.Title, OriginalTitle: movie.OriginalTitle, Year: movie.Year, Poster: movie.Poster, LibraryTitleID: linked[movie.ID]})
	}
	return out
}

func episodeViews(metadata []SeasonMetadata, videos []VideoView, now time.Time) ([]SeasonView, int) {
	seasons := make([]SeasonView, 0, len(metadata))
	missing := 0
	hasSpecials := false
	for _, season := range metadata {
		if season.Number == 0 {
			hasSpecials = true
		}
		view := SeasonView{Number: season.Number, Episodes: make([]EpisodeView, 0, len(season.Episodes))}
		for _, episode := range season.Episodes {
			files := make([]VideoView, 0)
			for _, video := range videos {
				if video.Place.Episode && video.Place.Season == season.Number && video.Place.First <= episode.Number && episode.Number <= video.Place.Last {
					files = append(files, video)
				}
			}
			status := EpisodePresent
			switch {
			case len(files) > 0:
			case episode.AirDate.IsZero() || episode.AirDate.After(now):
				status = EpisodeUpcoming
			default:
				status = EpisodeMissing
				if season.Number > 0 {
					missing++
				}
			}
			view.Episodes = append(view.Episodes, EpisodeView{ID: episode.ID, Number: episode.Number, Title: episode.Title, Overview: episode.Overview, Still: episode.Still, AirDate: episode.AirDate, Status: status, Files: files})
		}
		seasons = append(seasons, view)
	}
	if !hasSpecials {
		seasons = append([]SeasonView{{Number: 0, Episodes: []EpisodeView{}}}, seasons...)
	}
	return seasons, missing
}

// videoViews returns the live videos of a title in their order, each with its live subtitles.
func videoViews(title domain.LibraryTitle, files []domain.LibraryFile) []VideoView {
	subs := map[string][]SubtitleView{}
	for _, f := range files {
		if f.Kind != domain.FileSubtitle || !f.Live() {
			continue
		}
		key := strings.ToLower(f.VideoRel)
		subs[key] = append(subs[key], SubtitleView{ID: f.ID, Rel: f.Rel, Language: f.Language, Forced: f.Forced, SDH: f.SDH})
	}
	var out []VideoView
	for _, f := range files {
		if f.Kind != domain.FileVideo || !f.Live() {
			continue
		}
		v := VideoView{ID: f.ID, Rel: f.Rel, Size: f.Size, Place: f.Place, Version: f.Version, Subtitles: subs[strings.ToLower(f.Rel)]}
		if title.Kind == domain.KindSeries && title.TMDBID == 0 && f.Place.Episode {
			v.ChapterTitle = domain.ChapterTitleFromName(f.Rel, f.Place.Season, f.Place.First, f.Place.Last, f.Version)
		}
		out = append(out, v)
	}
	return out
}

// anyLive reports whether one of the files is out of the trash.
func anyLive(files []domain.LibraryFile) bool {
	for _, f := range files {
		if f.Live() {
			return true
		}
	}
	return false
}
