package domain

import (
	"errors"
	"slices"
	"testing"
)

const (
	ironFolder = "Movies/Iron Man (2008) [tmdbid-1726]"
	ironBase   = "Iron Man (2008) [tmdbid-1726]"
	batFolder  = "TV Shows/Breaking Bad (2008) [tmdbid-1396]"
)

var (
	ironTitle = Title{Name: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true}
	batTitle  = Title{Name: "Breaking Bad", Year: 2008, TMDBID: 1396, IDTag: true}
)

// mediaOf is a video of the given frame, codec and audio languages (an empty language is an untagged track).
func mediaOf(width, height int, codec string, languages ...string) VersionMedia {
	m := VersionMedia{Width: width, Height: height, Codec: codec}
	for _, l := range languages {
		m.Audio = append(m.Audio, AudioMedia{Language: l})
	}
	return m
}

// occupantOf is a video of the place with its version, its registry id and its subtitles.
func occupantOf(rel string, id int64, version string, media VersionMedia, subs ...Existing) Occupant {
	return Occupant{
		Video:     Existing{Rel: rel, ID: id, Size: 9, Version: version, VersionKnown: true},
		Subtitles: subs,
		Media:     media,
	}
}

func movieIncoming(source string, media VersionMedia, subs ...IncomingSubtitle) Incoming {
	return Incoming{
		Source: source, Kind: KindMovie, Root: "Movies", Title: ironTitle,
		Extension: ".mkv", Media: media, Subtitles: subs,
	}
}

func TestPlanDecisionTrashesOrRenamesAndPlacesTheNewVideo(t *testing.T) {
	t.Parallel()

	oldVideo := ironFolder + "/" + ironBase + ".mkv"
	oldSub := ironFolder + "/" + ironBase + ".en.srt"
	newVideo := ironFolder + "/" + ironBase + " - 1080p HEVC.mkv"
	newSub := ironFolder + "/" + ironBase + " - 1080p HEVC.es-419.srt"
	hevc := mediaOf(1920, 1080, "hevc", "eng")
	h264 := mediaOf(1920, 1080, "h264", "eng")
	tests := []struct {
		name      string
		decision  Decision
		occupants []Occupant
		exclusive bool
		incoming  []Incoming
		wantErr   error
		// wantTrash, wantRenames and wantPlace are the moves, as from→to (to is "" for the trash).
		wantTrash   []string
		wantRenames []string
		wantPlace   []string
		wantVersion string
	}{
		{
			name:     "skip changes nothing",
			decision: DecisionSkip,
			incoming: []Incoming{movieIncoming("_unassigned/x.mkv", hevc)},
		},
		{
			name:      "replace with no duplicate has nothing to replace",
			decision:  DecisionReplace,
			incoming:  []Incoming{movieIncoming("_unassigned/x.mkv", hevc)},
			wantErr:   ErrNoDuplicate,
			occupants: nil,
		},
		{
			name:      "replace sends the video and its subtitles to the trash and places the new one",
			decision:  DecisionReplace,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", h264, Existing{Rel: oldSub, ID: 2, Size: 1})},
			incoming: []Incoming{movieIncoming("_unassigned/a.mkv", hevc, IncomingSubtitle{
				Source: "_unassigned/a.es-419.srt", Language: "es-419", Extension: ".srt",
			})},
			wantTrash: []string{oldVideo + "→", oldSub + "→"},
			wantPlace: []string{
				"_unassigned/a.mkv→" + ironFolder + "/" + ironBase + ".mkv",
				"_unassigned/a.es-419.srt→" + ironFolder + "/" + ironBase + ".es-419.srt",
			},
		},
		{
			name:     "replace takes every version of the place, each video before its own subtitles",
			decision: DecisionReplace,
			occupants: []Occupant{
				occupantOf(oldVideo, 1, "", h264),
				occupantOf(ironFolder+"/"+ironBase+" - 2160p.mkv", 3, "2160p", mediaOf(3840, 2160, "hevc", "eng"), Existing{Rel: ironFolder + "/" + ironBase + " - 2160p.en.srt", ID: 4}),
			},
			incoming:  []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantTrash: []string{oldVideo + "→", ironFolder + "/" + ironBase + " - 2160p.mkv→", ironFolder + "/" + ironBase + " - 2160p.en.srt→"},
			wantPlace: []string{"_unassigned/a.mkv→" + ironFolder + "/" + ironBase + ".mkv"},
		},
		{
			name:      "replace takes a whole movie for its parts, and places every part",
			decision:  DecisionReplace,
			exclusive: true,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", h264)},
			incoming: []Incoming{
				{Source: "_unassigned/p1.mkv", Kind: KindMovie, Root: "Movies", Title: ironTitle, Place: Place{Part: 1}, Extension: ".mkv", Media: hevc},
				{Source: "_unassigned/p2.mkv", Kind: KindMovie, Root: "Movies", Title: ironTitle, Place: Place{Part: 2}, Extension: ".mkv", Media: hevc},
			},
			wantTrash: []string{oldVideo + "→"},
			wantPlace: []string{
				"_unassigned/p1.mkv→" + ironFolder + "/" + ironBase + " - part1.mkv",
				"_unassigned/p2.mkv→" + ironFolder + "/" + ironBase + " - part2.mkv",
			},
		},
		{
			name:      "replace refuses a version on the new video",
			decision:  DecisionReplace,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", h264)},
			incoming:  []Incoming{{Source: "_unassigned/a.mkv", Kind: KindMovie, Root: "Movies", Title: ironTitle, Extension: ".mkv", Version: "2160p"}},
			wantErr:   ErrInvalidLibrary,
		},
		{
			name:      "keep both: the bare video takes the label of its codec, the new one the other",
			decision:  DecisionKeepBoth,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", h264, Existing{Rel: oldSub, ID: 2, Size: 1})},
			incoming: []Incoming{movieIncoming("_unassigned/a.mkv", hevc, IncomingSubtitle{
				Source: "_unassigned/a.es-419.srt", Language: "es-419", Extension: ".srt",
			})},
			wantRenames: []string{
				oldVideo + "→" + ironFolder + "/" + ironBase + " - 1080p H.264.mkv",
				oldSub + "→" + ironFolder + "/" + ironBase + " - 1080p H.264.en.srt",
			},
			wantPlace: []string{
				"_unassigned/a.mkv→" + newVideo,
				"_unassigned/a.es-419.srt→" + newSub,
			},
			wantVersion: "1080p HEVC",
		},
		{
			name:        "keep both: a version the registry has is kept, and a new label equal to it gets (2)",
			decision:    DecisionKeepBoth,
			occupants:   []Occupant{occupantOf(oldVideo, 1, "1080p HEVC", h264)},
			incoming:    []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantPlace:   []string{"_unassigned/a.mkv→" + ironFolder + "/" + ironBase + " - 1080p HEVC (2).mkv"},
			wantVersion: "1080p HEVC (2)",
		},
		{
			name:      "keep both: an incomplete new label needs the user's",
			decision:  DecisionKeepBoth,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", hevc)},
			incoming:  []Incoming{movieIncoming("_unassigned/a.mkv", mediaOf(1920, 1080, "hevc", ""))},
			wantErr:   ErrIncompleteLabel,
		},
		{
			name:        "keep both: the label the user chose is taken",
			decision:    DecisionKeepBoth,
			occupants:   []Occupant{occupantOf(oldVideo, 1, "", hevc)},
			incoming:    []Incoming{{Source: "_unassigned/a.mkv", Kind: KindMovie, Root: "Movies", Title: ironTitle, Extension: ".mkv", Media: mediaOf(1920, 1080, "hevc", ""), Version: "1080p Dual"}},
			wantRenames: []string{oldVideo + "→" + ironFolder + "/" + ironBase + " - 1080p Inglés.mkv"},
			wantPlace:   []string{"_unassigned/a.mkv→" + ironFolder + "/" + ironBase + " - 1080p Dual.mkv"},
			wantVersion: "1080p Dual",
		},
		{
			name:      "keep both: a bare video whose label is incomplete needs the user's",
			decision:  DecisionKeepBoth,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", mediaOf(1920, 1080, "hevc", ""))},
			incoming:  []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantErr:   ErrIncompleteLabel,
		},
		{
			name:      "keep both: a file whose version cannot be read is not kept beside",
			decision:  DecisionKeepBoth,
			occupants: []Occupant{{Video: Existing{Rel: ironFolder + "/Iron Man.mkv"}, Media: h264}},
			incoming:  []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantErr:   ErrVersionUnknown,
		},
		{
			name:      "keep both is refused for parts and versions are exclusive",
			decision:  DecisionKeepBoth,
			exclusive: true,
			occupants: []Occupant{occupantOf(ironFolder+"/"+ironBase+" - part1.mkv", 1, "", h264)},
			incoming:  []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantErr:   ErrExclusive,
		},
		{
			name:      "keep both takes one new video",
			decision:  DecisionKeepBoth,
			occupants: []Occupant{occupantOf(oldVideo, 1, "", h264)},
			incoming:  []Incoming{movieIncoming("_unassigned/a.mkv", hevc), movieIncoming("_unassigned/b.mkv", hevc)},
			wantErr:   ErrInvalidLibrary,
		},
		{
			name:     "keep both with no duplicate has nothing to keep beside",
			decision: DecisionKeepBoth,
			incoming: []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantErr:  ErrNoDuplicate,
		},
		{
			name:     "a decision that does not exist is refused",
			decision: "rename",
			incoming: []Incoming{movieIncoming("_unassigned/a.mkv", hevc)},
			wantErr:  ErrInvalidLibrary,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanDecision(tt.decision, tt.occupants, tt.exclusive, tt.incoming)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("PlanDecision error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PlanDecision: %v", err)
			}
			if got.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", got.Version, tt.wantVersion)
			}
			if !slices.Equal(movesOf(got.Trash), tt.wantTrash) {
				t.Errorf("trash = %q, want %q", movesOf(got.Trash), tt.wantTrash)
			}
			if !slices.Equal(movesOf(got.Renames), tt.wantRenames) {
				t.Errorf("renames = %q, want %q", movesOf(got.Renames), tt.wantRenames)
			}
			if !slices.Equal(movesOf(got.Place), tt.wantPlace) {
				t.Errorf("place = %q, want %q", movesOf(got.Place), tt.wantPlace)
			}
		})
	}
}

// movesOf writes each move as from→to, to empty for a file that goes to the trash.
func movesOf(moves []Move) []string {
	var out []string
	for _, m := range moves {
		out = append(out, m.From+"→"+m.To)
	}
	return out
}

func TestPlanDecisionRenamesAChapterAndNamesItAsSpecSays(t *testing.T) {
	t.Parallel()

	oldRel := batFolder + "/Season 01/Breaking Bad (2008) - S01E02.mkv"
	plan, err := PlanDecision(DecisionKeepBoth,
		[]Occupant{occupantOf(oldRel, 0, "", mediaOf(1920, 1080, "h264", "eng"))},
		false,
		[]Incoming{{
			Source: "_unassigned/c.mkv", Kind: KindSeries, Root: "TV Shows", Title: batTitle,
			Place: Place{Episode: true, Season: 1, First: 2, Last: 2}, ChapterTitle: "Cat's in the Bag",
			Extension: ".mkv", Media: mediaOf(1920, 1080, "hevc", "eng"),
		}})
	if err != nil {
		t.Fatalf("PlanDecision: %v", err)
	}
	wantRename := batFolder + "/Season 01/Breaking Bad (2008) - S01E02 - 1080p H.264.mkv"
	wantPlace := "TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/Breaking Bad (2008) - S01E02 - Cat's in the Bag - 1080p HEVC.mkv"
	if len(plan.Renames) != 1 || plan.Renames[0].To != wantRename {
		t.Errorf("renames = %q, want %q", movesOf(plan.Renames), wantRename)
	}
	if len(plan.Place) != 1 || plan.Place[0].To != wantPlace {
		t.Errorf("place = %q, want %q", movesOf(plan.Place), wantPlace)
	}
}

func TestVersionOfAMovieFileComesFromItsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rel         string
		wantVersion string
		wantOK      bool
	}{
		{name: "the whole movie, named as its folder", rel: ironFolder + "/" + ironBase + ".mkv", wantOK: true},
		{name: "a version after the folder's name", rel: ironFolder + "/" + ironBase + " - 2160p.mkv", wantVersion: "2160p", wantOK: true},
		{name: "a part is not a version", rel: ironFolder + "/" + ironBase + " - part2.mkv", wantOK: true},
		{name: "a name without the folder's name cannot be read", rel: ironFolder + "/Iron Man.mkv"},
		{name: "a dash with no version cannot be read", rel: ironFolder + "/" + ironBase + " - .mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := MovieVersionOf(ironFolder, tt.rel)
			if ok != tt.wantOK || got != tt.wantVersion {
				t.Errorf("MovieVersionOf = %q, %v; want %q, %v", got, ok, tt.wantVersion, tt.wantOK)
			}
		})
	}
}

func TestVersionOfAChapterIsReadOnlyWhenItHasNoTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rel    string
		wantOK bool
	}{
		{name: "a chapter with only its code has no version", rel: batFolder + "/Season 01/Breaking Bad (2008) - S01E02.mkv", wantOK: true},
		{name: "two chapters with only their code have no version", rel: batFolder + "/Season 01/Breaking Bad (2008) - S01E01-E02.mkv", wantOK: true},
		{name: "a title may hold a version, so the name alone does not say", rel: batFolder + "/Season 01/Breaking Bad (2008) - S01E02 - Cat.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := SeriesVersionOf(tt.rel); ok != tt.wantOK {
				t.Errorf("SeriesVersionOf ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestSubtitleSuffixKeepsWhatFollowsTheVideoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		video  string
		sub    string
		want   string
		wantOK bool
	}{
		{name: "a language and flags", video: "M/Movie.mkv", sub: "M/Movie.en.forced.srt", want: ".en.forced.srt", wantOK: true},
		{name: "a language with a region", video: "M/Iron Man (2008).mkv", sub: "M/Iron Man (2008).es-419.srt", want: ".es-419.srt", wantOK: true},
		{name: "another name", video: "M/Movie.mkv", sub: "M/Other.srt"},
		{name: "a longer name that only starts the same", video: "M/Movie.mkv", sub: "M/Movie 2.srt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := SubtitleSuffix(tt.video, tt.sub)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("SubtitleSuffix = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
