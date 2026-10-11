package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

func TestStepInverseUndoesTheStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step domain.Step
		want domain.Step
	}{
		{
			name: "a rename goes back", step: domain.Step{Kind: domain.StepRename, From: "/lib/a.mkv", To: "/lib/_unassigned/a.mkv"},
			want: domain.Step{Kind: domain.StepRename, From: "/lib/_unassigned/a.mkv", To: "/lib/a.mkv"},
		},
		{
			name: "a folder created is removed", step: domain.Step{Kind: domain.StepMkdir, To: "/lib/_unassigned"},
			want: domain.Step{Kind: domain.StepRemoveDir, From: "/lib/_unassigned"},
		},
		{
			name: "a folder removed is created again", step: domain.Step{Kind: domain.StepRemoveDir, From: "/lib/old"},
			want: domain.Step{Kind: domain.StepMkdir, To: "/lib/old"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.step.Inverse(); got != tt.want {
				t.Errorf("Inverse() = %+v, want %+v", got, tt.want)
			}
			if got := tt.step.Inverse().Inverse(); got != tt.step {
				t.Errorf("Inverse twice = %+v, want %+v", got, tt.step)
			}
		})
	}
}

func TestPlanInverseRunsTheStepsBackwards(t *testing.T) {
	t.Parallel()

	plan := domain.Plan{Kind: "trash.put", Source: "job", Steps: []domain.Step{
		{Kind: domain.StepMkdir, To: "/lib/.videoexplorer/trash/x"},
		{Kind: domain.StepRename, From: "/lib/.videoexplorer/staging/a.zip", To: "/lib/.videoexplorer/trash/x/a.zip"},
	}}
	inv := plan.Inverse()
	want := []domain.Step{
		{Kind: domain.StepRename, From: "/lib/.videoexplorer/trash/x/a.zip", To: "/lib/.videoexplorer/staging/a.zip"},
		{Kind: domain.StepRemoveDir, From: "/lib/.videoexplorer/trash/x"},
	}
	if inv.Kind != plan.Kind || inv.Source != plan.Source || !slices.Equal(inv.Steps, want) {
		t.Errorf("Inverse() = %+v, want the steps %+v", inv, want)
	}
	if !slices.Equal(inv.Inverse().Steps, plan.Steps) {
		t.Errorf("Inverse of the inverse = %+v, want the original steps", inv.Inverse().Steps)
	}
}

func TestPlanValidateRefusesWhatCannotBePerformed(t *testing.T) {
	t.Parallel()

	rename := func(from, to string) domain.Step { return domain.Step{Kind: domain.StepRename, From: from, To: to} }
	mkdir := func(to string) domain.Step { return domain.Step{Kind: domain.StepMkdir, To: to} }
	tests := []struct {
		name  string
		plan  domain.Plan
		valid bool
	}{
		{name: "a plan of renames and folders", valid: true, plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{
			mkdir("/lib/a"), rename("/lib/x.mkv", "/lib/a/x.mkv"),
		}}},
		{name: "an empty plan is valid", valid: true, plan: domain.Plan{Kind: "k", Source: "s"}},
		{name: "no kind", plan: domain.Plan{Source: "s", Steps: []domain.Step{mkdir("/lib/a")}}},
		{name: "no source", plan: domain.Plan{Kind: "k", Steps: []domain.Step{mkdir("/lib/a")}}},
		{name: "a rename without a target", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{rename("/lib/x.mkv", "")}}},
		{name: "a rename onto itself", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{rename("/lib/x.mkv", "/lib/x.mkv")}}},
		{name: "a rename that only changes case", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{rename("/lib/Movie", "/lib/movie")}}},
		{name: "a folder with a path in both fields", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{{Kind: domain.StepMkdir, From: "/lib/a", To: "/lib/a"}}}},
		{name: "two steps create the same path", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{
			rename("/lib/a.mkv", "/lib/x.mkv"), rename("/lib/b.mkv", "/lib/X.MKV"),
		}}},
		{name: "a folder and a file with the same path", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{
			mkdir("/lib/x"), rename("/lib/a.mkv", "/lib/X"),
		}}},
		{name: "an unknown kind", plan: domain.Plan{Kind: "k", Source: "s", Steps: []domain.Step{{Kind: "copy", From: "a", To: "b"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.plan.Validate()
			if tt.valid && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !tt.valid && !errors.Is(err, domain.ErrInvalidPlan) {
				t.Fatalf("Validate = %v, want ErrInvalidPlan", err)
			}
		})
	}
}

func TestOperationUndoRevertsOnlyWhatMayHaveRun(t *testing.T) {
	t.Parallel()

	steps := []domain.Step{
		{Kind: domain.StepMkdir, To: "/lib/u"},
		{Kind: domain.StepRename, From: "/lib/s/a.mkv", To: "/lib/u/a.mkv"},
		{Kind: domain.StepRename, From: "/lib/s/b.mkv", To: "/lib/u/b.mkv"},
	}
	tests := []struct {
		name    string
		started int
		want    []domain.Step
	}{
		{name: "cut before the first step: nothing to revert", started: 0, want: nil},
		{name: "cut inside the first step: the step is reverted, it may have run", started: 1, want: []domain.Step{
			{Kind: domain.StepRemoveDir, From: "/lib/u"},
		}},
		{name: "cut after the first rename started: the rename and the folder are reverted", started: 2, want: []domain.Step{
			{Kind: domain.StepRename, From: "/lib/u/a.mkv", To: "/lib/s/a.mkv"},
			{Kind: domain.StepRemoveDir, From: "/lib/u"},
		}},
		{name: "cut after the last step, before the commit: everything is reverted", started: 3, want: []domain.Step{
			{Kind: domain.StepRename, From: "/lib/u/b.mkv", To: "/lib/s/b.mkv"},
			{Kind: domain.StepRename, From: "/lib/u/a.mkv", To: "/lib/s/a.mkv"},
			{Kind: domain.StepRemoveDir, From: "/lib/u"},
		}},
		{name: "a count past the plan does not go past its steps", started: 9, want: []domain.Step{
			{Kind: domain.StepRename, From: "/lib/u/b.mkv", To: "/lib/s/b.mkv"},
			{Kind: domain.StepRename, From: "/lib/u/a.mkv", To: "/lib/s/a.mkv"},
			{Kind: domain.StepRemoveDir, From: "/lib/u"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := domain.Operation{ID: "op", Plan: domain.Plan{Kind: "k", Source: "s", Steps: steps}, Started: tt.started, CreatedAt: time.Unix(0, 0)}
			if got := op.Undo().Steps; !slices.Equal(got, tt.want) {
				t.Errorf("Undo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
