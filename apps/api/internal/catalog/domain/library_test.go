package domain

import (
	"errors"
	"testing"
	"time"
)

var libNow = time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC)

func TestNewLibraryTitleChecksItsValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    TitleKind
		title   string
		disc    string
		year    int
		tmdbID  int64
		idTag   bool
		folder  string
		wantErr bool
	}{
		{name: "a linked movie with its tag", kind: KindMovie, title: "Iron Man", disc: "Iron Man", year: 2008, tmdbID: 1726, idTag: true, folder: "Movies/Iron Man (2008) [tmdbid-1726]"},
		{name: "a linked movie whose tag was removed", kind: KindMovie, title: "Iron Man", disc: "Iron Man", year: 2008, tmdbID: 1726, idTag: false, folder: "Movies/Iron Man (2008)"},
		{name: "a free series with no year", kind: KindSeries, title: "Home Videos", disc: "Home Videos", folder: "TV Shows/Home Videos"},
		{name: "a free title may not carry the tag", kind: KindMovie, title: "Free", disc: "Free", year: 2001, idTag: true, folder: "Movies/Free (2001)", wantErr: true},
		{name: "an unknown kind", kind: "documentary", title: "X", disc: "X", folder: "Movies/X", wantErr: true},
		{name: "an empty title", kind: KindMovie, title: "", disc: "X", folder: "Movies/X", wantErr: true},
		{name: "an empty disc title", kind: KindMovie, title: "X", disc: "", folder: "Movies/X", wantErr: true},
		{name: "a negative year", kind: KindMovie, title: "X", disc: "X", year: -1, folder: "Movies/X", wantErr: true},
		{name: "a year of five digits", kind: KindMovie, title: "X", disc: "X", year: 10000, folder: "Movies/X", wantErr: true},
		{name: "a negative TMDB id", kind: KindMovie, title: "X", disc: "X", tmdbID: -3, folder: "Movies/X", wantErr: true},
		{name: "a folder with no root", kind: KindMovie, title: "X", disc: "X", folder: "X", wantErr: true},
		{name: "a folder below the root", kind: KindMovie, title: "X", disc: "X", folder: "Movies/Sub/X", wantErr: true},
		{name: "a folder in the unassigned section", kind: KindMovie, title: "X", disc: "X", folder: "_unassigned/X", wantErr: true},
		{name: "a hidden root", kind: KindMovie, title: "X", disc: "X", folder: ".videoexplorer/X", wantErr: true},
		{name: "a folder that climbs out", kind: KindMovie, title: "X", disc: "X", folder: "Movies/../X", wantErr: true},
		{name: "an absolute folder", kind: KindMovie, title: "X", disc: "X", folder: "/Movies/X", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewLibraryTitle(tt.kind, tt.title, tt.disc, tt.year, tt.tmdbID, tt.idTag, tt.folder, libNow)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidLibrary) {
					t.Fatalf("NewLibraryTitle error = %v, want ErrInvalidLibrary", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewLibraryTitle: %v", err)
			}
			if got.Folder != tt.folder || got.Kind != tt.kind || got.IDTag != tt.idTag || !got.CreatedAt.Equal(libNow) {
				t.Errorf("title = %+v", got)
			}
		})
	}
}

func TestNewVideoChecksItsPlaceAndItsName(t *testing.T) {
	t.Parallel()

	whole := Place{}
	part2 := Place{Part: 2}
	special := Place{Episode: true, Season: 0, First: 1, Last: 1}
	double := Place{Episode: true, Season: 1, First: 1, Last: 2}

	tests := []struct {
		name    string
		kind    TitleKind
		spec    VideoSpec
		wantErr bool
	}{
		{name: "the whole movie", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A (2008)/A (2008).mkv", Place: whole, Size: 10}},
		{name: "a version of the whole movie", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A (2008)/A (2008) - 2160p.mkv", Place: whole, Version: "2160p", Size: 10}},
		{name: "a part of a movie", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A (2008)/A (2008) - part2.mkv", Place: part2, Size: 10}},
		{name: "a part with a version is refused (RF-22)", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A (2008)/x.mkv", Place: part2, Version: "2160p", Size: 10}, wantErr: true},
		{name: "a movie with chapters", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: special, Size: 10}, wantErr: true},
		{name: "a movie with a negative part", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: Place{Part: -1}, Size: 10}, wantErr: true},
		{name: "a special of a series", kind: KindSeries, spec: VideoSpec{Rel: "TV Shows/A/Season 00/x.mkv", Place: special, Size: 10}},
		{name: "two chapters in one file", kind: KindSeries, spec: VideoSpec{Rel: "TV Shows/A/Season 01/x.mkv", Place: double, Size: 10}},
		{name: "a series file that is not a chapter", kind: KindSeries, spec: VideoSpec{Rel: "TV Shows/A/x.mkv", Place: whole, Size: 10}, wantErr: true},
		{name: "a series chapter with no first chapter", kind: KindSeries, spec: VideoSpec{Rel: "TV Shows/A/x.mkv", Place: Place{Episode: true, Season: 1}, Size: 10}, wantErr: true},
		{name: "a series chapter that ends before it starts", kind: KindSeries, spec: VideoSpec{Rel: "TV Shows/A/x.mkv", Place: Place{Episode: true, Season: 1, First: 3, Last: 2}, Size: 10}, wantErr: true},
		{name: "a series file with a part", kind: KindSeries, spec: VideoSpec{Rel: "TV Shows/A/x.mkv", Place: Place{Episode: true, Season: 1, First: 1, Last: 1, Part: 1}, Size: 10}, wantErr: true},
		{name: "a path that climbs out", kind: KindMovie, spec: VideoSpec{Rel: "Movies/../x.mkv", Place: whole, Size: 10}, wantErr: true},
		{name: "a hidden component", kind: KindMovie, spec: VideoSpec{Rel: "Movies/.hidden/x.mkv", Place: whole, Size: 10}, wantErr: true},
		{name: "a negative size", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: whole, Size: -1}, wantErr: true},
		{name: "a hash of another algorithm", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: whole, PartialHash: "ph2:ab", Size: 10}, wantErr: true},
		{name: "a hash with the ph1 prefix", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: whole, PartialHash: "ph1:ab", Size: 10}},
		{name: "a version with a separator", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: whole, Version: "a/b", Size: 10}, wantErr: true},
		{name: "a version that is blank", kind: KindMovie, spec: VideoSpec{Rel: "Movies/A/x.mkv", Place: whole, Version: "  ", Size: 10}, wantErr: true},
		{name: "an unknown kind", kind: "book", spec: VideoSpec{Rel: "Books/x.mkv", Place: whole, Size: 10}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewVideo(tt.kind, tt.spec, libNow)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidLibrary) {
					t.Fatalf("NewVideo error = %v, want ErrInvalidLibrary", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewVideo: %v", err)
			}
			if got.Kind != FileVideo || got.Rel != tt.spec.Rel || got.Place != tt.spec.Place || got.VideoRel != "" {
				t.Errorf("video = %+v", got)
			}
		})
	}
}

func TestNewSubtitleNormalizesItsLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		spec     SubtitleSpec
		wantLang string
		wantErr  bool
	}{
		{name: "Latin American Spanish becomes es-419", spec: SubtitleSpec{Rel: "M/a.es-MX.srt", VideoRel: "M/a.mkv", Language: "es-MX"}, wantLang: "es-419"},
		{name: "a forced English subtitle", spec: SubtitleSpec{Rel: "M/a.en.forced.srt", VideoRel: "M/a.mkv", Language: "en", Forced: true}, wantLang: "en"},
		{name: "a SDH flag is kept", spec: SubtitleSpec{Rel: "M/a.en.sdh.srt", VideoRel: "M/a.mkv", Language: "en", SDH: true}, wantLang: "en"},
		{name: "a subtitle is not its own video", spec: SubtitleSpec{Rel: "M/a.srt", VideoRel: "M/a.srt", Language: "en"}, wantErr: true},
		{name: "an empty language", spec: SubtitleSpec{Rel: "M/a.srt", VideoRel: "M/a.mkv", Language: ""}, wantErr: true},
		{name: "a language that is not ISO 639-1", spec: SubtitleSpec{Rel: "M/a.srt", VideoRel: "M/a.mkv", Language: "spanish"}, wantErr: true},
		{name: "a video path that climbs out", spec: SubtitleSpec{Rel: "M/a.srt", VideoRel: "../a.mkv", Language: "en"}, wantErr: true},
		{name: "a negative size", spec: SubtitleSpec{Rel: "M/a.srt", VideoRel: "M/a.mkv", Language: "en", Size: -2}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewSubtitle(tt.spec, libNow)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidLibrary) {
					t.Fatalf("NewSubtitle error = %v, want ErrInvalidLibrary", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewSubtitle: %v", err)
			}
			if got.Kind != FileSubtitle || got.Language != tt.wantLang || got.VideoRel != tt.spec.VideoRel ||
				got.Forced != tt.spec.Forced || got.SDH != tt.spec.SDH {
				t.Errorf("subtitle = %+v; want language %q", got, tt.wantLang)
			}
		})
	}
}

func TestLiveFilesAreThoseOutOfTheTrash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file LibraryFile
		want bool
	}{
		{name: "a file in the library", file: LibraryFile{}, want: true},
		{name: "a file in the trash", file: LibraryFile{TrashEntryID: 4}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.file.Live(); got != tt.want {
				t.Errorf("Live() = %v, want %v", got, tt.want)
			}
		})
	}
}
