package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// The registry of the library (spec RF-21, RF-22, RF-23, RF-24, RF-45): the titles the user
// assigned and the videos and subtitles kept in their folders. The disk stays the source of
// the names; the registry says which file belongs to which title, and which of them are
// already in the trash. Everything here is pure: no I/O.

// TitleKind says whether a title is a movie (Movies/) or a series (TV Shows/), spec §2.
type TitleKind string

// The kinds of title.
const (
	KindMovie  TitleKind = "movie"
	KindSeries TitleKind = "series"
)

// FileKind says whether a registered file is a video or a subtitle, spec §2.
type FileKind string

// The kinds of file.
const (
	FileVideo    FileKind = "video"
	FileSubtitle FileKind = "subtitle"
)

// ErrInvalidLibrary is returned for a registry value that is not usable.
var ErrInvalidLibrary = errors.New("invalid library record")

// partialHashPrefix is the version prefix of a partial hash (RF-24, domain of ingestion).
const partialHashPrefix = "ph1:"

// LibraryTitle is a movie or a series of the library (RF-21, RF-45).
type LibraryTitle struct {
	ID   int64
	Kind TitleKind
	// Title is the title the user sees: the TMDB title, or the free title he typed (RF-21).
	Title string
	// DiscTitle is <T> of spec §5: the title the folder and the files are named with.
	DiscTitle string
	// Year is <A> of spec §5; 0 when unknown.
	Year int
	// TMDBID is the TMDB id of a linked title; 0 for a free title (spec §6).
	TMDBID int64
	// IDTag says whether the tag [tmdbid-ID] is on (RF-45). A free title never has it.
	IDTag bool
	// Folder is the title's folder inside the library, with "/" separators: its files live in it
	// (movies) or in its season folders (series).
	Folder    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewLibraryTitle checks the values of a title that is not stored yet.
func NewLibraryTitle(kind TitleKind, title, discTitle string, year int, tmdbID int64, idTag bool, folder string, now time.Time) (LibraryTitle, error) {
	switch {
	case kind != KindMovie && kind != KindSeries:
		return LibraryTitle{}, fmt.Errorf("%w: kind %q", ErrInvalidLibrary, kind)
	case title == "":
		return LibraryTitle{}, fmt.Errorf("%w: empty title", ErrInvalidLibrary)
	case discTitle == "":
		return LibraryTitle{}, fmt.Errorf("%w: empty disc title", ErrInvalidLibrary)
	case year < 0 || year > maxYear:
		return LibraryTitle{}, fmt.Errorf("%w: year %d", ErrInvalidLibrary, year)
	case tmdbID < 0:
		return LibraryTitle{}, fmt.Errorf("%w: TMDB id %d", ErrInvalidLibrary, tmdbID)
	case idTag && tmdbID == 0:
		return LibraryTitle{}, fmt.Errorf("%w: a free title has no tag of ID (RF-45)", ErrInvalidLibrary)
	case !titleFolder(folder):
		return LibraryTitle{}, fmt.Errorf("%w: folder %q", ErrInvalidLibrary, folder)
	}
	return LibraryTitle{
		Kind: kind, Title: title, DiscTitle: discTitle, Year: year, TMDBID: tmdbID, IDTag: idTag,
		Folder: folder, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// maxYear is the largest year the registry keeps (four digits).
const maxYear = 9999

// titleFolder reports whether folder is a title's folder: one folder under a library root,
// with a name that is not hidden and not the unassigned section or the app's own folder.
func titleFolder(folder string) bool {
	if !safeRelativePath(folder) || strings.Count(folder, "/") != 1 {
		return false
	}
	root, _, _ := strings.Cut(folder, "/")
	return root != UnassignedDir && !strings.HasPrefix(root, ".")
}

// CheckLibraryRel reports whether rel is a path inside the library that the registry can keep:
// safe components joined with "/", none of them hidden and none the app's own folder (RNF-02).
func CheckLibraryRel(rel string) error {
	if !safeRelativePath(rel) {
		return fmt.Errorf("%w: path %q", ErrInvalidLibrary, rel)
	}
	for part := range strings.SplitSeq(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("%w: hidden path %q", ErrInvalidLibrary, rel)
		}
	}
	return nil
}

// Place is where a video of a title sits (RF-22, spec §5). A chapter of a series holds the
// chapters First to Last of Season (season 0 holds the specials). A movie is the whole movie
// (Part 0) or one of its parts (RF-22). The subtitles have no place of their own: they take the
// place of their video.
type Place struct {
	// Episode is true for a chapter of a series, false for a movie or one of its parts.
	Episode bool
	// Season, First and Last are the season and the chapters of a series file.
	Season, First, Last int
	// Part is the number of a part of a movie; 0 for the whole movie.
	Part int
}

// VideoSpec is what the registry keeps of a video that is in the library.
type VideoSpec struct {
	// Rel is the video's path inside the library.
	Rel   string
	Place Place
	// Version is the <versión> of RF-24; "" for the only file of its place.
	Version string
	Size    int64
	// PartialHash is the partial hash of the video (RF-24), "" when it is not known.
	PartialHash string
}

// SubtitleSpec is what the registry keeps of a subtitle that goes with a video (RF-23).
type SubtitleSpec struct {
	Rel string
	// VideoRel is the path of the video the subtitle goes with, in the same title.
	VideoRel string
	// Language is the <idioma> of spec §5; it is normalized as the file name writes it.
	Language string
	Forced   bool
	SDH      bool
	Size     int64
}

// LibraryFile is a video or a subtitle of a title, as the registry keeps it (RF-22, RF-23, RF-24).
type LibraryFile struct {
	ID      int64
	TitleID int64
	// Rel is the file's path inside the library, with "/" separators.
	Rel  string
	Kind FileKind
	// Place is the place of a video; the zero value for a subtitle.
	Place   Place
	Version string
	// Language, Forced and SDH describe a subtitle (RF-23).
	Language string
	Forced   bool
	SDH      bool
	// VideoRel is the path of the video a subtitle goes with; "" for a video.
	VideoRel string
	// VideoID is the logical registry id a subtitle follows. It may outlive the video row while trashed.
	VideoID     int64
	Size        int64
	PartialHash string
	// TrashEntryID is the trash entry the file is in; 0 while it is in the library (RF-30).
	TrashEntryID int64
	// TrashRel keeps the original registry path while Rel is a unique trash sentinel.
	TrashRel  string
	CreatedAt time.Time
}

// Live reports whether the file is in the library and not in the trash.
func (f LibraryFile) Live() bool { return f.TrashEntryID == 0 }

// NewVideo checks a video of a title of kind and returns it as the registry keeps it.
func NewVideo(kind TitleKind, s VideoSpec, now time.Time) (LibraryFile, error) {
	if err := CheckLibraryRel(s.Rel); err != nil {
		return LibraryFile{}, err
	}
	if s.Size < 0 {
		return LibraryFile{}, fmt.Errorf("%w: negative size", ErrInvalidLibrary)
	}
	if s.PartialHash != "" && !strings.HasPrefix(s.PartialHash, partialHashPrefix) {
		return LibraryFile{}, fmt.Errorf("%w: partial hash %q", ErrInvalidLibrary, s.PartialHash)
	}
	if err := checkPlace(kind, s.Place, s.Version); err != nil {
		return LibraryFile{}, err
	}
	if s.Version != "" && !plainName(s.Version) {
		return LibraryFile{}, fmt.Errorf("%w: version %q", ErrInvalidLibrary, s.Version)
	}
	return LibraryFile{
		Rel: s.Rel, Kind: FileVideo, Place: s.Place, Version: s.Version, Size: s.Size,
		PartialHash: s.PartialHash, CreatedAt: now,
	}, nil
}

// checkPlace checks that a video's place fits its title's kind: chapters for series, a part
// or the whole movie for movies; and that a part and a version are not both given (RF-22).
func checkPlace(kind TitleKind, p Place, version string) error {
	switch kind {
	case KindMovie:
		if p.Episode || p.Season != 0 || p.First != 0 || p.Last != 0 {
			return fmt.Errorf("%w: a movie file has no chapters", ErrInvalidLibrary)
		}
		if p.Part < 0 {
			return fmt.Errorf("%w: part %d", ErrInvalidLibrary, p.Part)
		}
		if p.Part > 0 && version != "" {
			return fmt.Errorf("%w: a part and a version are exclusive (RF-22)", ErrInvalidLibrary)
		}
	case KindSeries:
		switch {
		case !p.Episode:
			return fmt.Errorf("%w: a series file is a chapter", ErrInvalidLibrary)
		case p.Part != 0:
			return fmt.Errorf("%w: a series has no parts", ErrInvalidLibrary)
		case p.Season < 0:
			return fmt.Errorf("%w: season %d", ErrInvalidLibrary, p.Season)
		case p.First < 1 || p.Last < p.First:
			return fmt.Errorf("%w: chapters %d to %d", ErrInvalidLibrary, p.First, p.Last)
		}
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidLibrary, kind)
	}
	return nil
}

// NewSubtitle checks a subtitle that goes with a video and returns it as the registry keeps it.
// The language is normalized as spec §5 writes it (NormalizeLanguage).
func NewSubtitle(s SubtitleSpec, now time.Time) (LibraryFile, error) {
	if err := CheckLibraryRel(s.Rel); err != nil {
		return LibraryFile{}, err
	}
	if err := CheckLibraryRel(s.VideoRel); err != nil {
		return LibraryFile{}, err
	}
	if s.Rel == s.VideoRel {
		return LibraryFile{}, fmt.Errorf("%w: a subtitle is not its own video", ErrInvalidLibrary)
	}
	if s.Size < 0 {
		return LibraryFile{}, fmt.Errorf("%w: negative size", ErrInvalidLibrary)
	}
	lang, err := NormalizeLanguage(s.Language)
	if err != nil {
		return LibraryFile{}, fmt.Errorf("%w: %w", ErrInvalidLibrary, err)
	}
	return LibraryFile{
		Rel: s.Rel, Kind: FileSubtitle, Language: lang, Forced: s.Forced, SDH: s.SDH,
		VideoRel: s.VideoRel, Size: s.Size, CreatedAt: now,
	}, nil
}

// plainName reports whether s can be a part of a file name: not empty, no separators, no control characters.
func plainName(s string) bool {
	return strings.TrimSpace(s) != "" && safeComponent(s)
}
