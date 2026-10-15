package application

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// Errors of Asignar that the interfaces answer with their own status (RF-26, RF-25).
var (
	// ErrEntryNotFound means there is no entry of No asignados with that id.
	ErrEntryNotFound = errors.New("the entry is not in No asignados")
	// ErrAssigning means another tab holds the entry: it is being assigned (RF-26).
	ErrAssigning = errors.New("the entry is being assigned")
	// ErrLockLost means this tab does not hold the entry's lock any more: it expired or was taken.
	ErrLockLost = errors.New("the assignment lock is not held")
	// ErrInvalidQuery means the title search has no text to look for.
	ErrInvalidQuery = errors.New("the search needs a title")
	// ErrPlanInvalid means the choices cannot be committed; the plan says what is wrong (RF-25).
	ErrPlanInvalid = errors.New("the assignment plan is not valid")
)

// The errors of the TMDB port. The composition root maps the metadata context's errors to them.
var (
	// ErrTMDBNotConfigured means no TMDB credentials are set: the assignment uses free names (spec §6).
	ErrTMDBNotConfigured = errors.New("TMDB is not configured")
	// ErrTMDBUnavailable means TMDB failed or could not be reached (spec §6).
	ErrTMDBUnavailable = errors.New("TMDB is not available")
	// ErrTMDBNotFound means TMDB does not know the title or the season asked for.
	ErrTMDBNotFound = errors.New("TMDB does not know it")
)

// TMDB is what Asignar asks of the metadata context (RF-20, RF-21, RF-22, spec §5, §6). Every
// answer comes from the response cache when it can (spec §6).
type TMDB interface {
	// Configured reports whether TMDB credentials are set.
	Configured() bool
	// SearchTitles returns the movies or series that match query, optionally from one year.
	SearchTitles(ctx context.Context, kind domain.TitleKind, query string, year int) ([]TMDBTitle, error)
	// Title returns a movie or a series with its disc title (spec §5, <T>) and its year.
	Title(ctx context.Context, kind domain.TitleKind, id int64) (TMDBTitle, error)
	// Chapters returns the chapter title of spec §5 for the chapters first to last of a season,
	// and the chapters TMDB does not know (RF-22). It answers ErrTMDBNotFound for an unknown season.
	Chapters(ctx context.Context, seriesID int64, season, first, last int) (TMDBChapters, error)
}

// TMDBTitle is a title as TMDB answers it, shaped for the naming rules of spec §5.
type TMDBTitle struct {
	ID    int64
	Title string
	// OriginalTitle is the title in its original language; empty when TMDB does not give it (RF-29 links by it).
	OriginalTitle string
	// DiscTitle is <T> of spec §5; empty in a search result.
	DiscTitle string
	// Year is <A>; 0 when unknown.
	Year int
	// PosterFile is the file name of the poster, as /api/images/poster serves it; "" when none.
	PosterFile string
}

// TMDBChapters is the chapter title of a file that holds some chapters of a season (spec §5).
type TMDBChapters struct {
	// Title is <título cap.>; "" when the chapters have no title.
	Title string
	// Missing lists the chapters that TMDB does not have in the season (RF-22).
	Missing []int
}

// NameReader reads what a file name, and its folders, say about the file (RF-20). The composition
// root adapts the ingestion's analysis of names.
type NameReader interface {
	Read(rel string, subtitle bool) NameReading
}

// NameReading is what a name says (RF-20, RF-23). A zero field is absent.
type NameReading struct {
	// Kind is the type the name tells: movie, series, or "" when it does not say (absolute
	// numbering, a generic name, §8).
	Kind     domain.TitleKind
	Title    string
	Year     int
	Season   *int
	Chapter  *int
	Last     *int
	Part     int
	Language string
	Forced   bool
	SDH      bool
}
