package application

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestMoveUnderKeepsTheRestOfThePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rel     string
		from    string
		to      string
		want    string
		wantErr bool
	}{
		{name: "a chapter moves with its folder", rel: "TV Shows/Lizzie (2008)/Season 01/bb.S01E02.mkv", from: "TV Shows/Lizzie (2008)", to: "TV Shows/Lizzie (2008) [tmdbid-1]", want: "TV Shows/Lizzie (2008) [tmdbid-1]/Season 01/bb.S01E02.mkv"},
		{name: "the case of the folder may differ", rel: "tv shows/lizzie (2008)/Season 01/a.mkv", from: "TV Shows/Lizzie (2008)", to: "TV Shows/Lizzie", want: "TV Shows/Lizzie/Season 01/a.mkv"},
		{name: "a subtitle of the folder itself", rel: "TV Shows/Lizzie (2008)/a.es.srt", from: "TV Shows/Lizzie (2008)", to: "TV Shows/Lizzie", want: "TV Shows/Lizzie/a.es.srt"},
		{name: "a file outside the folder is refused", rel: "TV Shows/Lizzie (2008) x/a.mkv", from: "TV Shows/Lizzie (2008)", to: "TV Shows/Lizzie", wantErr: true},
		{name: "the folder itself is not a file", rel: "TV Shows/Lizzie (2008)", from: "TV Shows/Lizzie (2008)", to: "TV Shows/Lizzie", wantErr: true},
		{name: "a file in another folder is refused", rel: "Movies/Iron Man (2008)/a.mkv", from: "TV Shows/Lizzie (2008)", to: "TV Shows/Lizzie", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := moveUnder(tt.rel, tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Fatalf("moveUnder(%q) err = %v, want error %t", tt.rel, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("moveUnder(%q) = %q, want %q", tt.rel, got, tt.want)
			}
		})
	}
}

func TestIDTagChoicesKeepTheChoicesOfTheFilesItDoesNotName(t *testing.T) {
	t.Parallel()

	st := editState{
		title: domain.LibraryTitle{ID: 7, Folder: "Movies/Iron Man (2008) [tmdbid-1726]"},
		files: []domain.LibraryFile{
			{ID: 1, Kind: domain.FileVideo, Rel: "a.mkv", Version: "1080p", Place: domain.Place{}},
			{ID: 2, Kind: domain.FileVideo, Rel: "b.mkv", Place: domain.Place{Part: 2}},
			{ID: 3, Kind: domain.FileSubtitle, Rel: "a.es.srt", VideoRel: "a.mkv", Language: "es-419", Forced: true},
		},
	}
	tests := []struct {
		name      string
		videos    []IDTagVideo
		wantVideo map[int64]EditVideo // the choices of the videos that the request names
		wantLen   int                 // videos in the edit, unknown answers included
	}{
		{name: "no answers: every file keeps its choices", wantVideo: map[int64]EditVideo{1: {FileID: 1, Version: "1080p"}, 2: {FileID: 2, Part: 2}}, wantLen: 2},
		{
			name:      "an answer replaces the duplicate answer and keeps the label without one",
			videos:    []IDTagVideo{{FileID: 1, Duplicate: domain.DecisionSkip}},
			wantVideo: map[int64]EditVideo{1: {FileID: 1, Version: "1080p", Duplicate: domain.DecisionSkip}},
			wantLen:   2,
		},
		{
			name:      "an answer gives the label of the version",
			videos:    []IDTagVideo{{FileID: 2, Version: "2160p"}},
			wantVideo: map[int64]EditVideo{2: {FileID: 2, Part: 2, Version: "2160p"}},
			wantLen:   2,
		},
		{name: "an answer for a file that is not a video is passed on to be refused", videos: []IDTagVideo{{FileID: 99}}, wantVideo: map[int64]EditVideo{1: {FileID: 1, Version: "1080p"}}, wantLen: 3},
		{name: "a second answer for one file is passed on to be refused", videos: []IDTagVideo{{FileID: 1, Version: "720p"}, {FileID: 1, Version: "480p"}}, wantVideo: map[int64]EditVideo{1: {FileID: 1, Version: "720p"}}, wantLen: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := idTagChoices(st, IDTagRequest{Revision: "r", IDTag: false, Videos: tt.videos})
			if len(req.Videos) != tt.wantLen {
				t.Fatalf("videos = %+v, want %d", req.Videos, tt.wantLen)
			}
			if len(req.Subtitles) != 1 || req.Subtitles[0].Language != "es-419" || !req.Subtitles[0].Forced {
				t.Errorf("the subtitle keeps its language and flags: %+v", req.Subtitles)
			}
			seen := map[int64]bool{}
			for _, got := range req.Videos {
				want, named := tt.wantVideo[got.FileID]
				if !named || seen[got.FileID] {
					continue
				}
				seen[got.FileID] = true
				if got.Version != want.Version || got.Duplicate != want.Duplicate || got.Part != want.Part {
					t.Errorf("video %d = %+v, want %+v", got.FileID, got, want)
				}
			}
		})
	}
}
