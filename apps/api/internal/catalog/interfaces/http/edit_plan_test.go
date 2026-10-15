package http

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
)

// TestTheFinalPathOfAnEditPlanIsOmittedWhileItWaits checks the contract of the edit preview (RF-24): a video or a
// subtitle with no final path yet (its duplicate waits for an answer) omits finalPath, as the assignment preview does,
// and a placed one carries its path.
func TestTheFinalPathOfAnEditPlanIsOmittedWhileItWaits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plan      application.EditPlan
		wantVideo *string
		wantSub   *string
	}{
		{
			name:      "a video and a subtitle waiting for their answer omit finalPath",
			plan:      application.EditPlan{Videos: []application.EditedVideo{{FileID: 1, File: "a.mkv"}}, Subtitles: []application.EditedSubtitle{{FileID: 2, File: "a.es.srt"}}},
			wantVideo: nil,
			wantSub:   nil,
		},
		{
			name: "placed files carry their final path",
			plan: application.EditPlan{
				Videos:    []application.EditedVideo{{FileID: 1, File: "a.mkv", FinalPath: "Movies/A/A.mkv", Changed: true}},
				Subtitles: []application.EditedSubtitle{{FileID: 2, File: "a.es.srt", FinalPath: "Movies/A/A.es.srt", Changed: true}},
			},
			wantVideo: strp("Movies/A/A.mkv"),
			wantSub:   strp("Movies/A/A.es.srt"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := editPlanToAPI(tt.plan)
			if got := out.Videos[0].FinalPath; !equalStrp(got, tt.wantVideo) {
				t.Errorf("video finalPath = %v, want %v", deref(got), deref(tt.wantVideo))
			}
			if got := out.Subtitles[0].FinalPath; !equalStrp(got, tt.wantSub) {
				t.Errorf("subtitle finalPath = %v, want %v", deref(got), deref(tt.wantSub))
			}
		})
	}
}

// strp returns a pointer to s.
func strp(s string) *string { return &s }

// equalStrp reports whether two optional strings are both absent or both equal.
func equalStrp(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// deref returns the string an optional one holds, or "<absent>".
func deref(s *string) string {
	if s == nil {
		return "<absent>"
	}
	return *s
}
