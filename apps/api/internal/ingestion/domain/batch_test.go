package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

func TestNewBatch(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		id        domain.BatchID
		batchName string
		relation  domain.Relation
		want      domain.Batch
		wantErr   bool
	}{
		{name: "single file without relation", id: "b1", relation: domain.RelationNone, want: domain.Batch{ID: "b1", Relation: domain.RelationNone, Origin: domain.OriginUpload, CreatedAt: now}},
		{name: "parts with a folder name", id: "b2", batchName: "  Iron Man  ", relation: domain.RelationParts, want: domain.Batch{ID: "b2", Name: "Iron Man", Relation: domain.RelationParts, Origin: domain.OriginUpload, CreatedAt: now}},
		{name: "episodes", id: "b3", batchName: "Breaking Bad S01", relation: domain.RelationEpisodes, want: domain.Batch{ID: "b3", Name: "Breaking Bad S01", Relation: domain.RelationEpisodes, Origin: domain.OriginUpload, CreatedAt: now}},
		{name: "unknown relation", id: "b4", relation: "r00", wantErr: true},
		{name: "empty relation", id: "b5", relation: "", wantErr: true},
		{name: "id with a slash", id: "a/b", relation: domain.RelationNone, wantErr: true},
		{name: "empty id", id: "", relation: domain.RelationNone, wantErr: true},
		{name: "id longer than 64", id: domain.BatchID(strings.Repeat("a", 65)), relation: domain.RelationNone, wantErr: true},
		{name: "name with a separator", id: "b6", batchName: "a/b", relation: domain.RelationNone, wantErr: true},
		{name: "name that is a parent folder", id: "b7", batchName: "..", relation: domain.RelationNone, wantErr: true},
		{name: "name with a control character", id: "b8", batchName: "a\tb", relation: domain.RelationNone, wantErr: true},
		{name: "name longer than 255 bytes", id: "b9", batchName: strings.Repeat("n", 256), relation: domain.RelationNone, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NewBatch(tt.id, tt.batchName, tt.relation, now)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidBatch) {
					t.Fatalf("err = %v, want ErrInvalidBatch", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *got != tt.want {
				t.Errorf("batch = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestRelationValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		relation domain.Relation
		want     bool
	}{
		{relation: domain.RelationParts, want: true},
		{relation: domain.RelationEpisodes, want: true},
		{relation: domain.RelationNone, want: true},
		{relation: "", want: false},
		{relation: "Parts", want: false},
		{relation: "multipart", want: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.relation), func(t *testing.T) {
			t.Parallel()

			if got := tt.relation.Valid(); got != tt.want {
				t.Errorf("Valid(%q) = %v, want %v", tt.relation, got, tt.want)
			}
		})
	}
}
