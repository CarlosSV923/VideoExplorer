package domain_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// file is a job of a unit for the tests: a file of the drop, with its origin path.
func file(id, name, origin string) *domain.UploadJob {
	return &domain.UploadJob{ID: domain.JobID(id), BatchID: "b1", FileName: name, RelativePath: origin}
}

// kept is a file the filter kept, at its place in the unit.
func kept(path string, kind domain.FileKind) domain.FilterEntry {
	return domain.FilterEntry{Path: path, Size: 10, Kind: kind}
}

// summary describes a plan as the tests compare it: its key, its name, whether it is
// loose, and its files as "rel<-path" (an empty side is left out).
func summary(p domain.EntryPlan) string {
	files := make([]string, 0, len(p.Files))
	for _, f := range p.Files {
		files = append(files, f.Rel+"<-"+f.Path)
	}
	return fmt.Sprintf("%s|%s|%t|%s", p.Key, p.Name, p.Loose, strings.Join(files, ","))
}

func summaries(plans []domain.EntryPlan) []string {
	out := make([]string, 0, len(plans))
	for _, p := range plans {
		out = append(out, summary(p))
	}
	return out
}

func TestPlanEntriesWithoutRelationMakesOneEntryPerUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		batch domain.Batch
		units []domain.UnitContent
		want  []string
	}{
		{
			name:  "a video uploaded on its own is a loose file",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.mkv", "")},
				Kept: []domain.FilterEntry{kept("Movie.mkv", domain.KindVideo)},
			}},
			want: []string{"j1|Movie.mkv|true|<-"},
		},
		{
			name:  "a subtitle on its own is a folder named after it",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.es.srt", "")},
				Kept: []domain.FilterEntry{kept("Movie.es.srt", domain.KindSubtitle)},
			}},
			want: []string{"j1|Movie.es|false|Movie.es.srt<-"},
		},
		{
			name:  "an archive is a folder with its content, even with a single video",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.zip", "")}, Extracted: true,
				Kept: []domain.FilterEntry{kept("Movie.mkv", domain.KindVideo)},
			}},
			want: []string{"j1|Movie|false|Movie.mkv<-Movie.mkv"},
		},
		{
			name:  "a video and its subtitle from an archive keep their folders",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Pack.rar", "")}, Extracted: true,
				Kept: []domain.FilterEntry{kept("Movie/Movie.mkv", domain.KindVideo), kept("Movie/Movie.es.srt", domain.KindSubtitle)},
			}},
			want: []string{"j1|Pack|false|Movie/Movie.mkv<-Movie/Movie.mkv,Movie/Movie.es.srt<-Movie/Movie.es.srt"},
		},
		{
			name:  "each unit of a batch without relation is its own entry",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{
				{Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "A.mkv", "")}, Kept: []domain.FilterEntry{kept("A.mkv", domain.KindVideo)}},
				{Leader: "j2", Jobs: []*domain.UploadJob{file("j2", "B.zip", "")}, Extracted: true, Kept: []domain.FilterEntry{kept("B.mkv", domain.KindVideo)}},
			},
			want: []string{"j1|A.mkv|true|<-", "j2|B|false|B.mkv<-B.mkv"},
		},
		{
			name:  "a folder-dropped video is loose, without the dropped folder",
			batch: domain.Batch{ID: "b1", Name: "Movie folder", Relation: domain.RelationNone},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.mkv", "Movie folder/Movie.mkv")},
				Kept: []domain.FilterEntry{kept("Movie folder/Movie.mkv", domain.KindVideo)},
			}},
			want: []string{"j1|Movie.mkv|true|<-"},
		},
		{
			name:  "a volume set is an archive named after its set",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.7z.001", ""), file("j2", "Movie.7z.002", "")}, Extracted: true,
				Kept: []domain.FilterEntry{kept("Movie.mkv", domain.KindVideo)},
			}},
			want: []string{"j1|Movie|false|Movie.mkv<-Movie.mkv"},
		},
		{
			name:  "a unit that kept nothing gives no entry",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationNone},
			units: []domain.UnitContent{
				{Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "notes.txt", "")}, Kept: []domain.FilterEntry{kept("notes.txt", domain.KindDiscard)}},
				{Leader: "j2", Jobs: []*domain.UploadJob{file("j2", "B.mkv", "")}, Kept: []domain.FilterEntry{kept("B.mkv", domain.KindVideo)}},
			},
			want: []string{"j2|B.mkv|true|<-"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := summaries(domain.PlanEntries(tt.batch, tt.units))
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("plans =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestPlanEntriesWithRelationMakesOneEntryForTheBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		batch domain.Batch
		units []domain.UnitContent
		want  []string
	}{
		{
			name:  "parts of a dropped folder: the folder is the entry and keeps its subfolders",
			batch: domain.Batch{ID: "b1", Name: "Movie", Relation: domain.RelationParts},
			units: []domain.UnitContent{
				{
					Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.mkv", "Movie/Movie.mkv")},
					Kept: []domain.FilterEntry{kept("Movie/Movie.mkv", domain.KindVideo)},
				},
				{
					Leader: "j2", Jobs: []*domain.UploadJob{file("j2", "Movie.es.srt", "Movie/sub/Movie.es.srt")},
					Kept: []domain.FilterEntry{kept("Movie/sub/Movie.es.srt", domain.KindSubtitle)},
				},
			},
			want: []string{"b1|Movie|false|Movie.mkv<-,sub/Movie.es.srt<-"},
		},
		{
			name:  "parts without a batch name take the name of their first archive set",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationParts},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.part1.rar", ""), file("j2", "Movie.part2.rar", "")}, Extracted: true,
				Kept: []domain.FilterEntry{kept("Movie.mkv", domain.KindVideo)},
			}},
			want: []string{"b1|Movie|false|Movie.mkv<-Movie.mkv"},
		},
		{
			name:  "episodes: an archive inside the dropped folder keeps its place in it",
			batch: domain.Batch{ID: "b1", Name: "Series", Relation: domain.RelationEpisodes},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "pack.zip", "Series/Season 01/pack.zip")}, Extracted: true,
				Kept: []domain.FilterEntry{kept("E01.mkv", domain.KindVideo)},
			}},
			want: []string{"b1|Series|false|Season 01/E01.mkv<-E01.mkv"},
		},
		{
			name:  "episodes: two archives share one entry at its root, named after the first",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationEpisodes},
			units: []domain.UnitContent{
				{Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "a.zip", "")}, Extracted: true, Kept: []domain.FilterEntry{kept("S01E01.mkv", domain.KindVideo)}},
				{Leader: "j2", Jobs: []*domain.UploadJob{file("j2", "b.zip", "")}, Extracted: true, Kept: []domain.FilterEntry{kept("S01E02.mkv", domain.KindVideo)}},
			},
			want: []string{"b1|a|false|S01E01.mkv<-S01E01.mkv,S01E02.mkv<-S01E02.mkv"},
		},
		{
			name:  "episodes: a loose upload joins the entry as a file, not as a loose entry",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationEpisodes},
			units: []domain.UnitContent{
				{Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "S01E02.mkv", "")}, Kept: []domain.FilterEntry{kept("S01E02.mkv", domain.KindVideo)}},
				{Leader: "j2", Jobs: []*domain.UploadJob{file("j2", "S01E01.mkv", "")}, Kept: []domain.FilterEntry{kept("S01E01.mkv", domain.KindVideo)}},
			},
			want: []string{"b1|S01E02|false|S01E02.mkv<-,S01E01.mkv<-"},
		},
		{
			name:  "a single video left by its batch is still a folder",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationParts},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "Movie.mkv", "")},
				Kept: []domain.FilterEntry{kept("Movie.mkv", domain.KindVideo)},
			}},
			want: []string{"b1|Movie|false|Movie.mkv<-"},
		},
		{
			name:  "a batch whose units kept nothing gives no entry",
			batch: domain.Batch{ID: "b1", Relation: domain.RelationParts},
			units: []domain.UnitContent{{
				Leader: "j1", Jobs: []*domain.UploadJob{file("j1", "a.txt", "")},
				Kept: []domain.FilterEntry{kept("a.txt", domain.KindDiscard)},
			}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := summaries(domain.PlanEntries(tt.batch, tt.units))
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("plans =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestEntryBaseIsTheNameWithoutArchiveOrExtension(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"Movie.zip":          "Movie",
		"Movie.7z.001":       "Movie",
		"Movie.zip.002":      "Movie",
		"Movie.part2.rar":    "Movie",
		"Movie.mkv":          "Movie",
		"Movie.2008.mkv":     "Movie.2008",
		"Movie":              "Movie",
		"Movie.tar.gz.final": "Movie.tar.gz",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := domain.EntryBase(name); got != want {
				t.Errorf("EntryBase(%q) = %q, want %q", name, got, want)
			}
		})
	}
}
