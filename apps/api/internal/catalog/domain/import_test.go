package domain_test

import (
	"path"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

var (
	importNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	importOld = importNow.Add(-time.Hour)
	mediaExts = domain.MediaExts{
		Video:    []string{".mkv", ".mp4", ".avi"},
		Subtitle: []string{".srt", ".ass"},
	}
	movieRoot  = domain.ImportRoot{Name: "Movies", Kind: domain.KindMovie}
	seriesRoot = domain.ImportRoot{Name: "TV Shows", Kind: domain.KindSeries}
)

// fileItem is a file of a folder as the disk lists it, modified long ago unless told otherwise.
func fileItem(rel string, mod time.Time) domain.Item {
	return domain.Item{Rel: rel, Size: 1000, ModTime: mod}
}

// folderOf is a title folder with the given files (paths inside it). The folders above each file are listed
// too, as the disk lists them.
func folderOf(name string, files ...string) domain.Folder {
	var items []domain.Item
	seen := map[string]bool{}
	for _, f := range files {
		for dir := path.Dir(f); dir != "." && !seen[dir]; dir = path.Dir(dir) {
			seen[dir] = true
			items = append(items, domain.Item{Rel: dir, IsDir: true, ModTime: importOld})
		}
		items = append(items, fileItem(f, importOld))
	}
	return domain.Folder{Name: name, Items: items}
}

// readingsOf returns a ReadName that answers from a table, with an empty reading for the rest.
func readingsOf(table map[string]domain.FileReading) domain.ReadName {
	return func(rel string, _ bool) domain.FileReading { return table[rel] }
}

func intPtr(n int) *int { return &n }

// episode is the reading of a chapter with one number.
func episode(season, chapter int) domain.FileReading {
	return domain.FileReading{Season: intPtr(season), Chapter: intPtr(chapter)}
}

// subtitle is the reading of a subtitle in a language.
func subtitle(lang string) domain.FileReading { return domain.FileReading{Language: lang} }

// baseInput is an import of one root with the given folders, with nothing registered and no lookups.
func baseInput(root domain.ImportRoot, folders ...domain.Folder) domain.ImportInput {
	return domain.ImportInput{
		Roots: []domain.DiskRoot{{Root: root, Entries: folders}},
		Read:  readingsOf(nil),
		Media: mediaExts,
		Now:   importNow,
	}
}

// registered is a title row with its files, all of them live.
func registered(id int64, root domain.ImportRoot, folder string, files ...domain.LibraryFile) domain.RegisteredTitle {
	return domain.RegisteredTitle{
		Title: domain.LibraryTitle{ID: id, Kind: root.Kind, Title: folder, DiscTitle: folder, Folder: root.Name + "/" + folder},
		Files: files,
	}
}

// liveVideo is a video row of a title, live in the library.
func liveVideo(id int64, rel string, place domain.Place, version string) domain.LibraryFile {
	return domain.LibraryFile{ID: id, Rel: rel, Kind: domain.FileVideo, Place: place, Version: version, Size: 1000}
}

func TestParseFolderName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want domain.FolderName
	}{
		{"title and year", "Godzilla (2014)", domain.FolderName{Title: "Godzilla", Year: 2014}},
		{"no year", "Downloads", domain.FolderName{Title: "Downloads"}},
		{"Jellyfin tag after the year", "Iron Man (2008) [tmdbid-1726]", domain.FolderName{Title: "Iron Man", Year: 2008, JellyfinID: 1726, TMDBID: 1726}},
		{"Jellyfin tag before the year", "Iron Man [tmdbid-1726] (2008)", domain.FolderName{Title: "Iron Man", Year: 2008, JellyfinID: 1726, TMDBID: 1726}},
		{"braces tag gives the TMDB id only", "Godzilla (2014) {tmdb-1234}", domain.FolderName{Title: "Godzilla", Year: 2014, TMDBID: 1234}},
		{"IMDb tag", "Alien Romulus (2024) [imdbid-tt18412256]", domain.FolderName{Title: "Alien Romulus", Year: 2024, IMDbID: "tt18412256"}},
		{"IMDb id in capitals is lower case", "Alien (1979) [IMDBID-TT0078748]", domain.FolderName{Title: "Alien", Year: 1979, IMDbID: "tt0078748"}},
		{"Jellyfin tag wins over braces", "Dune (2021) {tmdb-1} [tmdbid-438631]", domain.FolderName{Title: "Dune", Year: 2021, JellyfinID: 438631, TMDBID: 438631}},
		{"a year in the title stays", "Blade Runner 2049 (2017)", domain.FolderName{Title: "Blade Runner 2049", Year: 2017}},
		{"a tag alone is no title", "[tmdbid-1726]", domain.FolderName{JellyfinID: 1726, TMDBID: 1726}},
		{"a zero id is no id", "Foo [tmdbid-0]", domain.FolderName{Title: "Foo"}},
		{"a year-like title word is not a year", "1917", domain.FolderName{Title: "1917"}},
		{"a series title with a dash keeps the dash", "Monstruo - La historia de Lizzie Borden (2026)", domain.FolderName{Title: "Monstruo - La historia de Lizzie Borden", Year: 2026}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ParseFolderName(tt.in); got != tt.want {
				t.Errorf("ParseFolderName(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

// movieCase runs PlanImport on one movie folder and checks its plan.
type importCase struct {
	name  string
	input domain.ImportInput
	check func(t *testing.T, p domain.ImportPlan)
}

func runImportCases(t *testing.T, cases []importCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.check(t, domain.PlanImport(tt.input))
		})
	}
}

// newTitleOf returns the only new title of a plan, failing the test when there is not exactly one.
func newTitleOf(t *testing.T, p domain.ImportPlan) domain.NewTitle {
	t.Helper()
	if len(p.NewTitles) != 1 {
		t.Fatalf("new titles = %+v, want exactly one", p.NewTitles)
	}
	return p.NewTitles[0]
}

// filesRel lists the rels of files in order.
func filesRel(files []domain.LibraryFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Rel)
	}
	return out
}

// movedFrom lists the library paths moved into No asignados, in plan order.
func movedFrom(p domain.ImportPlan) []string {
	var out []string
	for _, m := range p.Moves {
		for _, f := range m.Files {
			out = append(out, f.From)
		}
	}
	slices.Sort(out)
	return out
}

func TestPlanImportMovies(t *testing.T) {
	t.Parallel()
	runImportCases(t, []importCase{
		{
			name:  "a folder with year and its movie is registered in place, with no renaming",
			input: baseInput(movieRoot, folderOf("Godzilla (2014)", "Godzilla (2014).mp4")),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if nt.Title.Folder != "Movies/Godzilla (2014)" || nt.Title.Kind != domain.KindMovie || nt.Title.Title != "Godzilla" || nt.Title.DiscTitle != "Godzilla" || nt.Title.Year != 2014 {
					t.Errorf("title = %+v", nt.Title)
				}
				if nt.Title.TMDBID != 0 || nt.Title.IDTag {
					t.Errorf("a title without lookup is free: %+v", nt.Title)
				}
				if got := filesRel(nt.Files); !slices.Equal(got, []string{"Movies/Godzilla (2014)/Godzilla (2014).mp4"}) {
					t.Errorf("files = %v", got)
				}
				if len(p.Moves) != 0 || len(p.RemoveDirs) != 0 {
					t.Errorf("nothing moves: moves=%v dirs=%v", p.Moves, p.RemoveDirs)
				}
			},
		},
		{
			name:  "an unnamed single video is the movie of its folder",
			input: baseInput(movieRoot, folderOf("Godzilla (2014)", "godzilla.2014.1080p.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if got := filesRel(newTitleOf(t, p).Files); !slices.Equal(got, []string{"Movies/Godzilla (2014)/godzilla.2014.1080p.mkv"}) {
					t.Errorf("files = %v", got)
				}
			},
		},
		{
			name:  "a movie with no year in the file is registered with the folder's year",
			input: baseInput(movieRoot, folderOf("Iron Man (2008)", "Iron Man.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.Year != 2008 {
					t.Errorf("year = %d", nt.Title.Year)
				}
			},
		},
		{
			name: "an IMDb tag links the folder to TMDB, and the Jellyfin tag is off",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Alien Romulus (2024) [imdbid-tt18412256]", "Alien Romulus (2024) [imdbid-tt18412256].mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/alien romulus (2024) [imdbid-tt18412256]": {
					ByIMDb: []domain.TMDBHit{{ID: 1000001, Title: "Alien: Romulus", Year: 2024}},
				}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if nt.Title.TMDBID != 1000001 || nt.Title.Title != "Alien: Romulus" || nt.Title.IDTag || nt.Title.DiscTitle != "Alien Romulus" {
					t.Errorf("title = %+v", nt.Title)
				}
				if len(p.Wanted) != 1 || p.Wanted[0].IMDbID != "tt18412256" {
					t.Errorf("wanted = %+v", p.Wanted)
				}
			},
		},
		{
			name: "a Jellyfin tag whose id TMDB confirms keeps the tag on",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Iron Man (2008) [tmdbid-1726]", "Iron Man (2008) [tmdbid-1726].mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/iron man (2008) [tmdbid-1726]": {
					ByID: &domain.TMDBHit{ID: 1726, Title: "Iron Man"},
				}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.TMDBID != 1726 || !nt.Title.IDTag {
					t.Errorf("title = %+v", nt.Title)
				}
			},
		},
		{
			name: "a braces tag links to TMDB and leaves the Jellyfin tag off",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Godzilla (2014) {tmdb-1234}", "Godzilla (2014).mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/godzilla (2014) {tmdb-1234}": {ByID: &domain.TMDBHit{ID: 1234, Title: "Godzilla"}}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.TMDBID != 1234 || nt.Title.IDTag {
					t.Errorf("title = %+v", nt.Title)
				}
			},
		},
		{
			name: "a tag TMDB does not know falls back to the one title that matches name and year",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Godzilla (2014) [tmdbid-1]", "Godzilla (2014).mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/godzilla (2014) [tmdbid-1]": {
					Search: []domain.TMDBHit{{ID: 99, Title: "Godzilla", Year: 2014}, {ID: 98, Title: "Godzilla", Year: 1998}},
				}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if nt.Title.TMDBID != 99 || nt.Title.IDTag {
					t.Errorf("title = %+v, want TMDB 99 with the tag off (the folder's tag is another id)", nt.Title)
				}
			},
		},
		{
			name: "two titles that match name and year leave the folder free",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Godzilla (2014)", "Godzilla (2014).mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/godzilla (2014)": {
					Search: []domain.TMDBHit{{ID: 1, Title: "Godzilla", Year: 2014}, {ID: 2, Title: "godzilla", OriginalTitle: "Godzilla", Year: 2014}},
				}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.TMDBID != 0 {
					t.Errorf("title = %+v, want free", nt.Title)
				}
			},
		},
		{
			name: "a title matches by its original name, and signs do not count",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Le Fabuleux Destin d'Amelie Poulain (2001)", "Le Fabuleux Destin d'Amelie Poulain (2001).mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/le fabuleux destin d'amelie poulain (2001)": {
					Search: []domain.TMDBHit{{ID: 194, Title: "Amélie", OriginalTitle: "Le Fabuleux Destin d'Amelie Poulain", Year: 2001}},
				}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.TMDBID != 194 {
					t.Errorf("title = %+v, want the original name to match", nt.Title)
				}
			},
		},
		{
			name: "an accent makes a different title, so the folder stays free",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Amelie (2001)", "Amelie (2001).mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/amelie (2001)": {
					Search: []domain.TMDBHit{{ID: 194, Title: "Amélie", Year: 2001}},
				}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.TMDBID != 0 {
					t.Errorf("title = %+v, want free", nt.Title)
				}
			},
		},
		{
			name: "an unavailable TMDB leaves the title free",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Godzilla (2014)", "Godzilla (2014).mkv"))
				in.Lookups = map[string]domain.Lookup{"movies/godzilla (2014)": {Unavailable: true, Search: []domain.TMDBHit{{ID: 1, Title: "Godzilla", Year: 2014}}}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Title.TMDBID != 0 || nt.Title.Title != "Godzilla" {
					t.Errorf("title = %+v", nt.Title)
				}
			},
		},
		{
			name:  "parts of a movie are registered with their numbers",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010) - part1.mkv", "Foo (2010) - part2.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if got := filesRel(nt.Files); len(got) != 2 || nt.Files[0].Place.Part != 1 || nt.Files[1].Place.Part != 2 {
					t.Errorf("files = %v with parts %+v", got, nt.Files)
				}
			},
		},
		{
			name:  "versions with their labels are kept together",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010) - 2160p.mkv", "Foo (2010) - 1080p HEVC.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if len(nt.Files) != 2 || nt.Files[0].Version != "1080p HEVC" || nt.Files[1].Version != "2160p" {
					t.Errorf("files = %+v, want both versions in name order", nt.Files)
				}
			},
		},
		{
			name:  "a whole movie and a version of it are both kept",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "Foo (2010) - 1080p.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); len(nt.Files) != 2 {
					t.Errorf("files = %+v, want the whole movie and its version", nt.Files)
				}
			},
		},
		{
			name:  "two whole movies in one folder are not told apart and both stay out",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "Foo (2010).mp4")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 {
					t.Errorf("new titles = %+v, want none", p.NewTitles)
				}
				if got := movedFrom(p); !slices.Equal(got, []string{"Movies/Foo (2010)/Foo (2010).mkv", "Movies/Foo (2010)/Foo (2010).mp4"}) {
					t.Errorf("moved = %v", got)
				}
			},
		},
		{
			name:  "unnamed videos in a folder with several are not interpreted",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "a.mkv", "b.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(movedFrom(p)) != 2 {
					t.Errorf("titles=%+v moved=%v", p.NewTitles, movedFrom(p))
				}
			},
		},
		{
			name:  "a part and a whole movie do not mix, and neither is taken",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "Foo (2010) - part1.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(movedFrom(p)) != 2 {
					t.Errorf("titles=%+v moved=%v, want both out", p.NewTitles, movedFrom(p))
				}
			},
		},
		{
			name: "subtitles follow their video by name, with their language",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "Foo (2010).es.srt", "Foo (2010).en.forced.srt"))
				in.Read = readingsOf(map[string]domain.FileReading{
					"Foo (2010).es.srt":        subtitle("es"),
					"Foo (2010).en.forced.srt": {Language: "en", Forced: true},
				})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if len(nt.Files) != 3 || nt.Files[0].Kind != domain.FileVideo {
					t.Fatalf("files = %+v, want the video first", nt.Files)
				}
				// Subtitles come in name order after their video: "en.forced" before "es".
				en, es := nt.Files[1], nt.Files[2]
				if en.Kind != domain.FileSubtitle || en.VideoRel != "Movies/Foo (2010)/Foo (2010).mkv" || en.Language != "en" || !en.Forced {
					t.Errorf("english subtitle = %+v", en)
				}
				if es.Kind != domain.FileSubtitle || es.VideoRel != "Movies/Foo (2010)/Foo (2010).mkv" || es.Language != "es" || es.Forced {
					t.Errorf("spanish subtitle = %+v", es)
				}
			},
		},
		{
			name: "a subtitle with no language or no video is not registered",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "Foo (2010).srt", "Other.es.srt"))
				in.Read = readingsOf(map[string]domain.FileReading{"Other.es.srt": subtitle("es")})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if got := movedFrom(p); !slices.Equal(got, []string{"Movies/Foo (2010)/Foo (2010).srt", "Movies/Foo (2010)/Other.es.srt"}) {
					t.Errorf("moved = %v", got)
				}
			},
		},
		{
			name: "an archive in a movie folder goes to No asignados and empties the folder",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).rar"))
				in.Archives = map[string]bool{"movies/foo (2010)/foo (2010).rar": true}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 {
					t.Errorf("a folder with only an archive is no title")
				}
				if len(p.Moves) != 1 || p.Moves[0].Name != "Foo (2010)" || p.Moves[0].Source != "library:Movies/Foo (2010)" {
					t.Errorf("moves = %+v", p.Moves)
				}
			},
		},
		{
			name: "an archive recognized by its bytes is an archive whatever its name",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "pack.dat"))
				in.Archives = map[string]bool{"movies/foo (2010)/pack.dat": true}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if got := movedFrom(p); !slices.Equal(got, []string{"Movies/Foo (2010)/pack.dat"}) {
					t.Errorf("moved = %v", got)
				}
			},
		},
		{
			name: "a file that is not media stays where it is, and the folder stays",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).rar", "Foo (2010).nfo"))
				in.Archives = map[string]bool{"movies/foo (2010)/foo (2010).rar": true}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.RemoveDirs) != 0 || len(movedFrom(p)) != 1 {
					t.Errorf("RemoveDirs = %v moved = %v, the folder keeps the nfo", p.RemoveDirs, movedFrom(p))
				}
			},
		},
		{
			name:  "a video in a subfolder of a movie is not placed and its folder is emptied",
			input: baseInput(movieRoot, folderOf("Foo (2010)", "Extras/making.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 {
					t.Errorf("titles = %+v", p.NewTitles)
				}
				if !slices.Equal(p.RemoveDirs, []string{"Movies/Foo (2010)/Extras", "Movies/Foo (2010)"}) {
					t.Errorf("RemoveDirs = %v", p.RemoveDirs)
				}
			},
		},
		{
			name:  "a folder with no title has its media moved",
			input: baseInput(movieRoot, folderOf("[tmdbid-1726]", "movie.mkv")),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(movedFrom(p)) != 1 {
					t.Errorf("titles=%+v moved=%v", p.NewTitles, movedFrom(p))
				}
			},
		},
		{
			name: "a loose video in the root is no title and goes to No asignados as an entry of its own",
			input: domain.ImportInput{
				Roots: []domain.DiskRoot{{Root: movieRoot, Entries: []domain.Folder{{Name: "loose.mkv", Loose: true, Items: []domain.Item{fileItem("", importOld)}}}}},
				Read:  readingsOf(nil), Media: mediaExts, Now: importNow,
			},
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.Moves) != 1 || !p.Moves[0].Loose || p.Moves[0].Name != "loose.mkv" || p.Moves[0].Files[0].From != "Movies/loose.mkv" {
					t.Errorf("moves = %+v", p.Moves)
				}
			},
		},
	})
}

func TestPlanImportSeries(t *testing.T) {
	t.Parallel()
	runImportCases(t, []importCase{
		{
			name: "chapters in season folders are registered with season and chapter, and the names stay",
			input: func() domain.ImportInput {
				in := baseInput(seriesRoot, folderOf("Breaking Bad (2008)", "Season 01/Breaking Bad - S01E01 - Pilot.mkv", "Season 01/Breaking Bad - S01E02.mkv"))
				in.Read = readingsOf(map[string]domain.FileReading{
					"Season 01/Breaking Bad - S01E01 - Pilot.mkv": episode(1, 1),
					"Season 01/Breaking Bad - S01E02.mkv":         episode(1, 2),
				})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if nt.Title.Kind != domain.KindSeries || nt.Title.Folder != "TV Shows/Breaking Bad (2008)" {
					t.Errorf("title = %+v", nt.Title)
				}
				if len(nt.Files) != 2 || nt.Files[0].Place != (domain.Place{Episode: true, Season: 1, First: 1, Last: 1}) {
					t.Errorf("files = %+v", nt.Files)
				}
			},
		},
		{
			name: "a scene name with a year and a release group is read by its marker",
			input: func() domain.ImportInput {
				f := "Season 01/Monstruo.La.historia.de.Lizzie.Borden.S01E07.2026.1080P-Dual-Lat.mkv"
				in := baseInput(seriesRoot, folderOf("Monstruo - La historia de Lizzie Borden (2026)", f))
				in.Read = readingsOf(map[string]domain.FileReading{f: episode(1, 7)})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if nt.Title.Title != "Monstruo - La historia de Lizzie Borden" || nt.Title.Year != 2026 || len(nt.Files) != 1 {
					t.Errorf("title = %+v files = %+v", nt.Title, nt.Files)
				}
			},
		},
		{
			name: "chapters with no season folder and no marker, and specials in a season 0 folder",
			input: func() domain.ImportInput {
				in := baseInput(seriesRoot, folderOf("Show (2010)", "Season 01/Pilot.mkv", "Specials/Making.mkv", "Show.S01E02.mkv"))
				in.Read = readingsOf(map[string]domain.FileReading{
					"Season 01/Pilot.mkv": {Season: intPtr(1)},
					"Specials/Making.mkv": episode(0, 1),
					"Show.S01E02.mkv":     episode(1, 2),
				})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				nt := newTitleOf(t, p)
				if got := filesRel(nt.Files); !slices.Equal(got, []string{"TV Shows/Show (2010)/Show.S01E02.mkv", "TV Shows/Show (2010)/Specials/Making.mkv"}) {
					t.Errorf("files = %v", got)
				}
				if got := movedFrom(p); !slices.Equal(got, []string{"TV Shows/Show (2010)/Season 01/Pilot.mkv"}) {
					t.Errorf("moved = %v", got)
				}
			},
		},
		{
			name: "a file with several chapters keeps its range",
			input: func() domain.ImportInput {
				f := "Season 01/Show.S01E01E02.mkv"
				in := baseInput(seriesRoot, folderOf("Show (2010)", f))
				last := 2
				in.Read = readingsOf(map[string]domain.FileReading{f: {Season: intPtr(1), Chapter: intPtr(1), Last: &last}})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if nt := newTitleOf(t, p); nt.Files[0].Place != (domain.Place{Episode: true, Season: 1, First: 1, Last: 2}) {
					t.Errorf("place = %+v", nt.Files[0].Place)
				}
			},
		},
		{
			name: "two files for the same chapter are both out: versions are not imported",
			input: func() domain.ImportInput {
				a, b := "Season 01/Show.S01E01.1080p.mkv", "Season 01/Show.S01E01.720p.mkv"
				in := baseInput(seriesRoot, folderOf("Show (2010)", a, b))
				in.Read = readingsOf(map[string]domain.FileReading{a: episode(1, 1), b: episode(1, 1)})
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(movedFrom(p)) != 2 {
					t.Errorf("titles=%+v moved=%v", p.NewTitles, movedFrom(p))
				}
			},
		},
		{
			name: "an archive in a series goes to No asignados, and the folder left empty goes too",
			input: func() domain.ImportInput {
				in := baseInput(seriesRoot, folderOf("Monstruo - La historia de Jeffrey Dahmer (2022)", "a1b2.rar", "c3d4.rar"))
				in.Archives = map[string]bool{
					"tv shows/monstruo - la historia de jeffrey dahmer (2022)/a1b2.rar": true,
					"tv shows/monstruo - la historia de jeffrey dahmer (2022)/c3d4.rar": true,
				}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(p.Moves) != 1 || len(p.Moves[0].Files) != 2 {
					t.Fatalf("titles=%+v moves=%+v", p.NewTitles, p.Moves)
				}
				if !slices.Equal(p.RemoveDirs, []string{"TV Shows/Monstruo - La historia de Jeffrey Dahmer (2022)"}) {
					t.Errorf("RemoveDirs = %v", p.RemoveDirs)
				}
			},
		},
		{
			name: "a chapter of a registered series is added to its title, and an archive in it is moved",
			input: func() domain.ImportInput {
				in := baseInput(seriesRoot, folderOf("Show (2010)", "Season 01/Show.S01E01.mkv", "Season 01/Show.S01E02.mkv", "Season 01/pack.rar"))
				in.Titles = []domain.RegisteredTitle{registered(7, seriesRoot, "Show (2010)", liveVideo(1, "TV Shows/Show (2010)/Season 01/Show.S01E01.mkv", domain.Place{Episode: true, Season: 1, First: 1, Last: 1}, ""))}
				in.Read = readingsOf(map[string]domain.FileReading{"Season 01/Show.S01E02.mkv": episode(1, 2)})
				in.Archives = map[string]bool{"tv shows/show (2010)/season 01/pack.rar": true}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.AddFiles) != 1 || p.AddFiles[0].TitleID != 7 || len(p.AddFiles[0].Files) != 1 {
					t.Errorf("add files = %+v", p.AddFiles)
				}
				if got := movedFrom(p); !slices.Equal(got, []string{"TV Shows/Show (2010)/Season 01/pack.rar"}) {
					t.Errorf("moved = %v", got)
				}
			},
		},
	})
}

func TestPlanImportRegistryAndDisk(t *testing.T) {
	t.Parallel()
	movieRow := func(id int64, rel string, version string) domain.LibraryFile {
		return domain.LibraryFile{ID: id, Rel: rel, Kind: domain.FileVideo, Version: version, Size: 1000}
	}
	runImportCases(t, []importCase{
		{
			name: "a second scan over what the first one registered changes nothing",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Godzilla (2014)", "Godzilla (2014).mp4"))
				in.Titles = []domain.RegisteredTitle{registered(1, movieRoot, "Godzilla (2014)", movieRow(10, "Movies/Godzilla (2014)/Godzilla (2014).mp4", ""))}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles)+len(p.AddFiles)+len(p.Moves)+len(p.GoneFiles)+len(p.GoneTitles)+len(p.RemoveDirs)+len(p.Waiting) != 0 {
					t.Errorf("a second scan must change nothing: %+v", p)
				}
			},
		},
		{
			name: "a registered file that is gone leaves the registry, and its title with it when nothing is left",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot)
				in.Titles = []domain.RegisteredTitle{registered(1, movieRoot, "Godzilla (2014)", movieRow(10, "Movies/Godzilla (2014)/Godzilla (2014).mp4", ""))}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if !slices.Equal(p.GoneFiles, []int64{10}) || !slices.Equal(p.GoneTitles, []int64{1}) {
					t.Errorf("gone files=%v titles=%v", p.GoneFiles, p.GoneTitles)
				}
			},
		},
		{
			name: "a title keeps its title when another of its files is still there",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv", "Foo (2010) - 1080p.mkv"))
				in.Titles = []domain.RegisteredTitle{registered(1, movieRoot, "Foo (2010)",
					movieRow(10, "Movies/Foo (2010)/Foo (2010).mkv", ""),
					movieRow(11, "Movies/Foo (2010)/Foo (2010) - 2160p.mkv", "2160p"),
				)}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if !slices.Equal(p.GoneFiles, []int64{11}) || len(p.GoneTitles) != 0 {
					t.Errorf("gone files=%v titles=%v", p.GoneFiles, p.GoneTitles)
				}
				if len(p.AddFiles) != 1 || p.AddFiles[0].TitleID != 1 || p.AddFiles[0].Files[0].Version != "1080p" {
					t.Errorf("the new version joins its title: %+v", p.AddFiles)
				}
			},
		},
		{
			name: "a subtitle goes with its video when the video is gone, and the subtitle leaves the registry",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot)
				in.Titles = []domain.RegisteredTitle{registered(1, movieRoot, "Foo (2010)",
					movieRow(10, "Movies/Foo (2010)/Foo (2010).mkv", ""),
					domain.LibraryFile{ID: 11, Rel: "Movies/Foo (2010)/Foo (2010).es.srt", Kind: domain.FileSubtitle, Language: "es", VideoRel: "Movies/Foo (2010)/Foo (2010).mkv"},
				)}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if !slices.Equal(p.GoneFiles, []int64{11, 10}) {
					t.Errorf("gone = %v, want the subtitle before its video", p.GoneFiles)
				}
			},
		},
		{
			name: "a subtitle still on disk whose video is gone is then an unregistered file that moves",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).es.srt"))
				in.Read = readingsOf(map[string]domain.FileReading{"Foo (2010).es.srt": subtitle("es")})
				in.Titles = []domain.RegisteredTitle{registered(1, movieRoot, "Foo (2010)",
					movieRow(10, "Movies/Foo (2010)/Foo (2010).mkv", ""),
				)}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if !slices.Equal(p.GoneFiles, []int64{10}) || !slices.Equal(movedFrom(p), []string{"Movies/Foo (2010)/Foo (2010).es.srt"}) {
					t.Errorf("gone=%v moved=%v", p.GoneFiles, movedFrom(p))
				}
			},
		},
		{
			name: "a trashed file is not gone while it is in the trash, and its path is not taken again",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot, folderOf("Foo (2010)", "Foo (2010).mkv"))
				trashed := movieRow(10, "Movies/Foo (2010)/Foo (2010).mkv", "")
				trashed.TrashEntryID = 5
				in.Titles = []domain.RegisteredTitle{registered(1, movieRoot, "Foo (2010)", trashed)}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.GoneFiles) != 0 || len(p.NewTitles) != 0 || len(p.AddFiles) != 0 {
					t.Errorf("a trashed file keeps its place: %+v", p)
				}
				if got := movedFrom(p); !slices.Equal(got, []string{"Movies/Foo (2010)/Foo (2010).mkv"}) {
					t.Errorf("moved = %v", got)
				}
			},
		},
		{
			name:  "a title whose folder vanished is gone with its files",
			input: baseInput(movieRoot),
			check: func(t *testing.T, p domain.ImportPlan) {
				in := domain.ImportInput{Roots: []domain.DiskRoot{{Root: movieRoot}}, Read: readingsOf(nil), Media: mediaExts, Now: importNow}
				in.Titles = []domain.RegisteredTitle{registered(3, movieRoot, "Gone (2001)", movieRow(30, "Movies/Gone (2001)/Gone (2001).mkv", ""))}
				if got := domain.PlanImport(in); !slices.Equal(got.GoneTitles, []int64{3}) {
					t.Errorf("gone titles = %v", got.GoneTitles)
				}
			},
		},
		{
			name: "a title under a root the scan does not read is left alone",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot)
				in.Titles = []domain.RegisteredTitle{{Title: domain.LibraryTitle{ID: 4, Kind: domain.KindSeries, Folder: "Other/Show (2010)"}, Files: []domain.LibraryFile{movieRow(40, "Other/Show (2010)/x.mkv", "")}}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.GoneFiles) != 0 || len(p.GoneTitles) != 0 {
					t.Errorf("gone = %v titles = %v", p.GoneFiles, p.GoneTitles)
				}
			},
		},
		{
			name: "a file still being copied waits for a later scan, and so does its folder",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot)
				in.Roots = []domain.DiskRoot{{Root: movieRoot, Entries: []domain.Folder{{Name: "Foo (2010)", Items: []domain.Item{
					fileItem("Foo (2010).mkv", importNow.Add(-10*time.Second)),
				}}}}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(p.Moves) != 0 {
					t.Errorf("a copy in progress is not touched: %+v", p)
				}
				if len(p.Waiting) != 1 || p.Waiting[0].Rel != "Movies/Foo (2010)/Foo (2010).mkv" || p.Waiting[0].Stamp.Size != 1000 {
					t.Errorf("waiting = %+v", p.Waiting)
				}
			},
		},
		{
			name: "a file that changed since the last scan is still being copied",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot)
				in.Roots = []domain.DiskRoot{{Root: movieRoot, Entries: []domain.Folder{{Name: "Foo (2010)", Items: []domain.Item{
					{Rel: "Foo (2010).mkv", Size: 2000, ModTime: importOld},
				}}}}}
				in.Seen = map[string]domain.Stamp{"movies/foo (2010)/foo (2010).mkv": {Size: 1000, ModTime: importOld}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 0 || len(p.Waiting) != 1 {
					t.Errorf("titles=%+v waiting=%+v", p.NewTitles, p.Waiting)
				}
			},
		},
		{
			name: "a file that did not change since the last scan is processed",
			input: func() domain.ImportInput {
				in := baseInput(movieRoot)
				in.Roots = []domain.DiskRoot{{Root: movieRoot, Entries: []domain.Folder{{Name: "Foo (2010)", Items: []domain.Item{
					{Rel: "Foo (2010).mkv", Size: 1000, ModTime: importOld},
				}}}}}
				in.Seen = map[string]domain.Stamp{"movies/foo (2010)/foo (2010).mkv": {Size: 1000, ModTime: importOld}}
				return in
			}(),
			check: func(t *testing.T, p domain.ImportPlan) {
				if len(p.NewTitles) != 1 || len(p.Waiting) != 0 {
					t.Errorf("titles=%+v waiting=%+v", p.NewTitles, p.Waiting)
				}
			},
		},
	})
}

func TestPlanImportIsDeterministic(t *testing.T) {
	t.Parallel()
	in := baseInput(movieRoot,
		folderOf("B (2001)", "B (2001).mkv"),
		folderOf("A (2000)", "A (2000).mkv", "A (2000).rar"),
	)
	first := domain.PlanImport(in)
	second := domain.PlanImport(in)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("two runs over the same input differ:\n%+v\n%+v", first, second)
	}
}
