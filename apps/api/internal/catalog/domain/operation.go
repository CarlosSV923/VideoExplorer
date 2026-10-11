package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidPlan is returned for a plan whose steps cannot be performed as written.
var ErrInvalidPlan = errors.New("invalid library plan")

// StepKind is what one step of a library change does to the file system.
type StepKind string

const (
	// StepMkdir creates one folder. Its parent must exist: the planner creates the missing ones first.
	StepMkdir StepKind = "mkdir"
	// StepRename moves a file or a folder to a path that is free (RF-25: nothing is overwritten).
	StepRename StepKind = "rename"
	// StepRemoveDir removes one folder, which must be empty.
	StepRemoveDir StepKind = "rmdir"
)

// Step is one change of the file system, in the order of its plan. A mkdir step names
// its folder in To, a rmdir step in From, and a rename step moves From to To.
type Step struct {
	Kind StepKind `json:"kind"`
	From string   `json:"from,omitempty"`
	To   string   `json:"to,omitempty"`
}

// Inverse returns the step that undoes s: a rename goes back, a folder that was created
// is removed, and a folder that was removed is created again.
func (s Step) Inverse() Step {
	switch s.Kind {
	case StepRename:
		return Step{Kind: StepRename, From: s.To, To: s.From}
	case StepMkdir:
		return Step{Kind: StepRemoveDir, From: s.To}
	case StepRemoveDir:
		return Step{Kind: StepMkdir, To: s.From}
	default:
		return s
	}
}

// Plan is what a library change does, step by step (RF-25, RNF-04). Nothing is performed
// until the plan is journaled.
type Plan struct {
	// Kind says what the change is, for the journal and the log (for example "trash.put").
	Kind string
	// Source says what the change is for: the file it moves, or the entry it adds to.
	Source string
	Steps  []Step
}

// Validate checks that every step has the paths its kind needs, that no two steps create
// the same path (names are compared without case, as an SMB share does), and that a
// rename never moves a path onto itself or onto another case of itself.
func (p Plan) Validate() error {
	if p.Kind == "" || p.Source == "" {
		return fmt.Errorf("%w: kind and source are required", ErrInvalidPlan)
	}
	created := map[string]bool{}
	for i, s := range p.Steps {
		switch s.Kind {
		case StepMkdir:
			if s.To == "" || s.From != "" {
				return stepError(i, s, "a folder to create")
			}
			if err := claim(created, i, s.To); err != nil {
				return err
			}
		case StepRemoveDir:
			if s.From == "" || s.To != "" {
				return stepError(i, s, "a folder to remove")
			}
		case StepRename:
			if s.From == "" || s.To == "" || strings.EqualFold(s.From, s.To) {
				return stepError(i, s, "a path to move to another, different name")
			}
			if err := claim(created, i, s.To); err != nil {
				return err
			}
		default:
			return stepError(i, s, "a known kind")
		}
	}
	return nil
}

// claim marks a path as created by step i, or refuses it when an earlier step created it.
func claim(created map[string]bool, i int, path string) error {
	key := strings.ToLower(path)
	if created[key] {
		return fmt.Errorf("%w: step %d creates %q, which an earlier step already creates", ErrInvalidPlan, i, path)
	}
	created[key] = true
	return nil
}

func stepError(i int, s Step, want string) error {
	return fmt.Errorf("%w: step %d (%s) needs %s", ErrInvalidPlan, i, s.Kind, want)
}

// Inverse returns the plan that undoes p: its steps inverted, last first.
func (p Plan) Inverse() Plan {
	out := Plan{Kind: p.Kind, Source: p.Source, Steps: make([]Step, 0, len(p.Steps))}
	for i := len(p.Steps) - 1; i >= 0; i-- {
		out.Steps = append(out.Steps, p.Steps[i].Inverse())
	}
	return out
}

// Operation is a plan on its way. The journal keeps it from before its first step until
// the catalog records of the change are committed (RNF-04).
type Operation struct {
	ID   string
	Plan Plan
	// Started counts the steps begun, in order. It is recorded before each step runs, so
	// the steps that may have run are exactly the first Started ones.
	Started   int
	CreatedAt time.Time
}

// Attempted returns the steps that may have run: the ones begun.
func (o Operation) Attempted() []Step {
	n := min(max(o.Started, 0), len(o.Plan.Steps))
	return o.Plan.Steps[:n]
}

// Undo returns the plan that reverts what the operation may have changed: the inverse of
// its attempted steps, last first. A step that never ran finds nothing to revert, so the
// plan is safe whichever step the process stopped at.
func (o Operation) Undo() Plan {
	return Plan{Kind: o.Plan.Kind, Source: o.Plan.Source, Steps: Plan{Steps: o.Attempted()}.Inverse().Steps}
}
