package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

const (
	decisionIronFolder = "Movies/Iron Man (2008) [tmdbid-1726]"
	decisionIronBase   = "Iron Man (2008) [tmdbid-1726]"
	decisionIncoming   = "_unassigned/Iron/a.mkv"
)

// mediaByName answers the media of a file by its base name; a file it does not know has no media.
type mediaByName map[string]domain.VersionMedia

func (m mediaByName) Media(_ context.Context, p string) (domain.VersionMedia, error) {
	return m[filepath.Base(p)], nil
}

// scenario is a place of the library, a new video for it and the decision taken.
type scenario struct {
	kind   domain.TitleKind
	folder string
	// disk holds the files of the library, by path inside it, with their content.
	disk map[string]string
	// registry holds the registered files of the title, with their ids (1, 2…, in order).
	registry []domain.LibraryFile
	// media is what the labels read of each file, by base name.
	media map[string]domain.VersionMedia

	incomingMedia domain.VersionMedia
	// incomingSubs are the new subtitles, by path inside the library, with their language.
	incomingSubs map[string]string
	place        domain.Place
	chapterTitle string
	parts        bool
	decision     domain.Decision
	version      string
}

// runDecision builds the library of s, decides on its duplicates and returns the fixture, which
// holds the resulting library and the records written, or the error of the decision.
func runDecision(t *testing.T, s scenario) (*fixture, error) {
	t.Helper()
	f := newFixture(t)
	for rel, content := range s.disk {
		writeFile(t, filepath.Join(f.root, filepath.FromSlash(rel)), content)
	}
	writeFile(t, filepath.Join(f.root, filepath.FromSlash(decisionIncoming)), "incoming")
	for rel := range s.incomingSubs {
		writeFile(t, filepath.Join(f.root, filepath.FromSlash(rel)), "sub")
	}
	if len(s.registry) > 0 {
		err := f.journal.Commit(t.Context(), "", func(r application.Records) error {
			id, err := r.InsertLibraryTitle(t.Context(), domain.LibraryTitle{Kind: s.kind, Title: "Iron Man", DiscTitle: "Iron Man", Folder: s.folder})
			if err != nil {
				return err
			}
			return r.InsertLibraryFiles(t.Context(), id, s.registry)
		})
		if err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	repo := &libRepo{title: domain.LibraryTitle{ID: 1, Folder: s.folder, Kind: s.kind}, files: s.registry}
	finder := application.NewDuplicateFinder(repo, libraryfs.New(f.root), &hashes{root: f.root}, []string{".mkv", ".mp4"})
	target := domain.Target{Kind: s.kind, Parts: s.parts, Place: s.place}
	dups, err := finder.Find(t.Context(), s.folder, target, decisionIncoming)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	occupants, err := finder.Occupants(t.Context(), s.folder, dups, []string{".srt"}, mediaByName(s.media))
	if err != nil {
		t.Fatalf("Occupants: %v", err)
	}
	in := domain.Incoming{
		Source: decisionIncoming, Kind: s.kind, Root: "Movies", Title: domain.Title{Name: "Iron Man", Year: 2008, TMDBID: 1726, IDTag: true},
		Place: s.place, ChapterTitle: s.chapterTitle, Extension: ".mkv", Media: s.incomingMedia, Version: s.version,
	}
	if s.kind == domain.KindSeries {
		in.Root = "TV Shows"
		in.Title = domain.Title{Name: "Breaking Bad", Year: 2008, TMDBID: 1396, IDTag: true}
	}
	for rel, lang := range s.incomingSubs {
		in.Subtitles = append(in.Subtitles, domain.IncomingSubtitle{Source: rel, Language: lang, Extension: ".srt"})
	}
	plan, err := domain.PlanDecision(s.decision, occupants, dups.Exclusive, []domain.Incoming{in})
	if err != nil {
		return f, err
	}
	return f, engine(f.files, f.journal).Decide(t.Context(), plan, decisionIncoming, nil)
}

func TestDecisionsChangeTheLibraryAsPlanned(t *testing.T) {
	t.Parallel()

	hevc := domain.VersionMedia{Width: 1920, Height: 1080, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "eng"}}}
	h264 := domain.VersionMedia{Width: 1920, Height: 1080, Codec: "h264", Audio: []domain.AudioMedia{{Language: "eng"}}}
	old := decisionIronFolder + "/" + decisionIronBase + ".mkv"
	oldSub := decisionIronFolder + "/" + decisionIronBase + ".en.srt"
	newSub := "_unassigned/Iron/a.es-419.srt"
	tests := []struct {
		name string
		s    scenario
		// want lists the files of the library afterwards, sorted.
		want []string
		// wantTrash is the number of files the trash holds for the new entry, 0 when nothing went there.
		wantTrash int
		// wantErr is the error of the decision, nil when it is applied.
		wantErr error
		// check runs more assertions on the fixture after the decision.
		check func(t *testing.T, f *fixture)
	}{
		{
			name: "replace trashes the video and its subtitle and places the new video with its subtitle",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk:          map[string]string{old: "old", oldSub: "s"},
				media:         map[string]domain.VersionMedia{filepath.Base(old): h264},
				incomingMedia: hevc,
				incomingSubs:  map[string]string{newSub: "es-419"},
				decision:      domain.DecisionReplace,
			},
			want: []string{
				decisionIronFolder + "/" + decisionIronBase + ".es-419.srt",
				decisionIronFolder + "/" + decisionIronBase + ".mkv",
			},
			wantTrash: 2,
			check: func(t *testing.T, f *fixture) {
				t.Helper()
				if len(f.store.trash) != 1 || f.store.trash[0].Origin != domain.TrashOriginLibrary || f.store.trash[0].Reason != domain.TrashReasonReplaced {
					t.Errorf("trash = %+v; want one entry from the library, replaced", f.store.trash)
				}
				if got := readFile(t, filepath.Join(f.root, filepath.FromSlash(decisionIronFolder), decisionIronBase+".mkv")); got != "incoming" {
					t.Errorf("the new video = %q, want the incoming one", got)
				}
			},
		},
		{
			name: "replace takes every version of the place",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk: map[string]string{
					old: "old",
					decisionIronFolder + "/" + decisionIronBase + " - 2160p.mkv": "uhd",
				},
				media: map[string]domain.VersionMedia{
					filepath.Base(old):                h264,
					decisionIronBase + " - 2160p.mkv": {Width: 3840, Height: 2160, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "eng"}}},
				},
				incomingMedia: hevc,
				decision:      domain.DecisionReplace,
			},
			want:      []string{decisionIronFolder + "/" + decisionIronBase + ".mkv"},
			wantTrash: 2,
		},
		{
			name: "keep both renames the bare video and its subtitle, and places the new one with its label",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk: map[string]string{old: "old", oldSub: "s"},
				registry: []domain.LibraryFile{
					{ID: 1, Rel: old, Kind: domain.FileVideo, Size: 3},
					{ID: 2, Rel: oldSub, Kind: domain.FileSubtitle, Language: "en", Size: 1},
				},
				media:         map[string]domain.VersionMedia{filepath.Base(old): h264},
				incomingMedia: hevc,
				decision:      domain.DecisionKeepBoth,
			},
			want: []string{
				decisionIronFolder + "/" + decisionIronBase + " - 1080p H.264.en.srt",
				decisionIronFolder + "/" + decisionIronBase + " - 1080p H.264.mkv",
				decisionIronFolder + "/" + decisionIronBase + " - 1080p HEVC.mkv",
			},
			check: func(t *testing.T, f *fixture) {
				t.Helper()
				// The registry follows the renames: its rows name the files where they are now.
				if f.store.files[0].Rel != decisionIronFolder+"/"+decisionIronBase+" - 1080p H.264.mkv" ||
					f.store.files[1].Rel != decisionIronFolder+"/"+decisionIronBase+" - 1080p H.264.en.srt" {
					t.Errorf("registered files after the renames = %q, %q", f.store.files[0].Rel, f.store.files[1].Rel)
				}
			},
		},
		{
			name: "keep both gives the new label (2) when the registry already has that label",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk: map[string]string{old: "old"},
				registry: []domain.LibraryFile{
					{ID: 1, Rel: old, Kind: domain.FileVideo, Version: "1080p HEVC", Size: 3},
				},
				media:         map[string]domain.VersionMedia{filepath.Base(old): h264},
				incomingMedia: hevc,
				decision:      domain.DecisionKeepBoth,
			},
			want: []string{
				decisionIronFolder + "/" + decisionIronBase + " - 1080p HEVC (2).mkv",
				old,
			},
		},
		{
			name: "keep both reads the version of a file only the disk has, and gives the new label (2) when it is the same",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk: map[string]string{
					decisionIronFolder + "/" + decisionIronBase + " - 1080p HEVC.mkv": "kept",
				},
				media: map[string]domain.VersionMedia{
					decisionIronBase + " - 1080p HEVC.mkv": h264,
				},
				incomingMedia: hevc,
				decision:      domain.DecisionKeepBoth,
			},
			want: []string{
				decisionIronFolder + "/" + decisionIronBase + " - 1080p HEVC (2).mkv",
				decisionIronFolder + "/" + decisionIronBase + " - 1080p HEVC.mkv",
			},
		},
		{
			name: "keep both with an incomplete label is refused until the user completes it",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk:          map[string]string{old: "old"},
				media:         map[string]domain.VersionMedia{filepath.Base(old): hevc},
				incomingMedia: domain.VersionMedia{Width: 1920, Height: 1080, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "und"}}},
				decision:      domain.DecisionKeepBoth,
			},
			wantErr: domain.ErrIncompleteLabel,
			want:    []string{old, decisionIncoming},
		},
		{
			name: "keep both with the label the user completed",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk:          map[string]string{old: "old"},
				media:         map[string]domain.VersionMedia{filepath.Base(old): hevc},
				incomingMedia: domain.VersionMedia{Width: 1920, Height: 1080, Codec: "hevc", Audio: []domain.AudioMedia{{Language: "und"}}},
				decision:      domain.DecisionKeepBoth,
				version:       "1080p Dual",
			},
			want: []string{
				decisionIronFolder + "/" + decisionIronBase + " - 1080p Inglés.mkv",
				decisionIronFolder + "/" + decisionIronBase + " - 1080p Dual.mkv",
			},
		},
		{
			name: "keep both is refused, and the library is left as it was, when a version cannot be read",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk:          map[string]string{decisionIronFolder + "/Iron Man.mkv": "old"},
				media:         map[string]domain.VersionMedia{"Iron Man.mkv": h264},
				incomingMedia: hevc,
				decision:      domain.DecisionKeepBoth,
			},
			wantErr: domain.ErrVersionUnknown,
			want:    []string{decisionIncoming, decisionIronFolder + "/Iron Man.mkv"},
		},
		{
			name: "skip leaves the library and the records as they are",
			s: scenario{
				kind: domain.KindMovie, folder: decisionIronFolder,
				disk:          map[string]string{old: "old"},
				media:         map[string]domain.VersionMedia{filepath.Base(old): h264},
				incomingMedia: hevc,
				decision:      domain.DecisionSkip,
			},
			want: []string{decisionIncoming, old},
		},
		{
			name: "a chapter that has a duplicate is kept beside it with its label",
			s: scenario{
				kind: domain.KindSeries, folder: "TV Shows/Breaking Bad (2008) [tmdbid-1396]",
				disk: map[string]string{
					"TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/Breaking Bad (2008) - S01E02.mkv": "old",
				},
				media: map[string]domain.VersionMedia{
					"Breaking Bad (2008) - S01E02.mkv": h264,
				},
				incomingMedia: hevc,
				place:         domain.Place{Episode: true, Season: 1, First: 2, Last: 2},
				chapterTitle:  "Cat's in the Bag",
				decision:      domain.DecisionKeepBoth,
			},
			want: []string{
				"TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/Breaking Bad (2008) - S01E02 - 1080p H.264.mkv",
				"TV Shows/Breaking Bad (2008) [tmdbid-1396]/Season 01/Breaking Bad (2008) - S01E02 - Cat's in the Bag - 1080p HEVC.mkv",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := runDecision(t, tt.s)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("decision error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("decision: %v", err)
			}
			got := withoutMeta(libraryFiles(t, f.root))
			if !slices.Equal(got, sortedCopy(tt.want)) {
				t.Errorf("library files = %q, want %q", got, sortedCopy(tt.want))
			}
			if tt.wantTrash > 0 && len(f.store.trash) == 0 {
				t.Errorf("no trash entry was recorded, want %d files in the trash", tt.wantTrash)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return out
}

// withoutMeta drops the app's own folder (the trash and the operations) from a listing of the library.
func withoutMeta(files []string) []string {
	var out []string
	for _, f := range files {
		if !strings.HasPrefix(f, ".videoexplorer/") {
			out = append(out, f)
		}
	}
	return out
}
