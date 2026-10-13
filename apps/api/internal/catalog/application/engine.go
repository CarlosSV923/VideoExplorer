package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// A library change (RNF-04, RF-25) is planned, journaled, performed and recorded:
//
//  1. Do plans it: the steps, from what the library holds now, changing nothing;
//  2. the plan is saved in the journal before its first step runs;
//  3. each step is recorded as started, then performed;
//  4. the catalog records of the change are committed in the same transaction that
//     forgets the journal entry.
//
// If a step or the commit fails, the steps that may have run are undone at once. If
// the process stops in between, Recover undoes them on the next start. The catalog
// records are written only by the commit, so an interrupted change never has any.
// Nothing is ever overwritten: a step whose target exists is refused.

var (
	// ErrTargetTaken means a change would overwrite something that already exists (RF-25).
	ErrTargetTaken = errors.New("the target already exists")
	// ErrNotEmpty means a folder that should be removed still has something in it.
	ErrNotEmpty = errors.New("the folder is not empty")
	// ErrUndoFailed means a change could not be fully reverted. Its journal entry is kept
	// and the next start tries again.
	ErrUndoFailed = errors.New("library change could not be undone")
	// errBothExist means an interrupted move left the file at both of its places; nothing
	// is deleted to choose between them.
	errBothExist = errors.New("both places of an interrupted move exist")
)

// Files is the file system the catalog changes: the library, with its trash and its
// operations' scratch area, which live on the same dataset so that a move is a rename
// (RNF-04). Every path is built by the server; the adapter refuses anything outside the
// library and anything reached through a symbolic link.
type Files interface {
	// TrashPath is the path of an entry of the trash, given its elements.
	TrashPath(elem ...string) string
	// UnassignedPath is the path of an element of the unassigned section (_unassigned/).
	UnassignedPath(elem ...string) string
	// OpsPath is the scratch path of the operations (.videoexplorer/ops), given its elements.
	OpsPath(elem ...string) string
	// Mkdir creates one folder whose parent exists. It fails when the folder exists.
	Mkdir(dir string) error
	// RemoveDir removes one folder, which must be empty (ErrNotEmpty otherwise).
	RemoveDir(dir string) error
	// Move renames from to to; across file systems it copies and then deletes. It never
	// replaces an existing file.
	Move(from, to string) error
	// RemoveCopy deletes the partial copy that a cut move left at path, if there is one.
	RemoveCopy(path string) error
	// RemoveAll deletes a path inside the library, if it exists.
	RemoveAll(path string) error
	// Names lists the names in a folder inside the library; none when the folder is missing.
	Names(dir string) ([]string, error)
	// Exists reports whether anything (a file or a folder) is at path inside the library.
	Exists(path string) (bool, error)
}

// Journal stores the operations of the library until their change is committed.
type Journal interface {
	// Save stores an operation before its first step runs.
	Save(ctx context.Context, op domain.Operation) error
	// Advance records how many steps have started.
	Advance(ctx context.Context, id string, started int) error
	// Pending returns the operations that no commit closed, oldest first.
	Pending(ctx context.Context) ([]domain.Operation, error)
	// Forget deletes an operation that was undone, recording nothing.
	Forget(ctx context.Context, id string) error
	// Commit runs record in one transaction together with the deletion of the operation:
	// either both happen or neither does.
	Commit(ctx context.Context, id string, record func(Records) error) error
}

// Records writes the catalog side of a change. Only a Journal's Commit gives it a transaction.
type Records interface {
	// InsertTrash stores a trash entry with its files and returns its id.
	InsertTrash(ctx context.Context, entry domain.TrashEntry) (int64, error)
	// InsertUnassigned stores an entry of No asignados with its files and returns its id.
	InsertUnassigned(ctx context.Context, entry domain.UnassignedEntry) (int64, error)
	// AddUnassignedFiles adds files to an entry of No asignados that exists.
	AddUnassignedFiles(ctx context.Context, entryID int64, files []domain.UnassignedFile) error
	// InsertLibraryTitle stores a title of the library and returns its id (RF-21).
	InsertLibraryTitle(ctx context.Context, title domain.LibraryTitle) (int64, error)
	// InsertLibraryFiles stores files of a title, in order. A subtitle's video must be stored
	// before it, in the same change or earlier (RF-23).
	InsertLibraryFiles(ctx context.Context, titleID int64, files []domain.LibraryFile) error
	// MoveLibraryFile records the new path of a registered file that the change renamed.
	MoveLibraryFile(ctx context.Context, id int64, rel string) error
	// TrashLibraryFiles records that the files are in the trash entry entryID (RF-30).
	TrashLibraryFiles(ctx context.Context, ids []int64, entryID int64) error
}

// Engine performs the changes of the library. Changes run one at a time: planning reads
// the names of the library and of the journal, and no other change may take them in
// between.
type Engine struct {
	files   Files
	journal Journal
	now     func() time.Time
	newID   func() string
	log     *slog.Logger
	mu      sync.Mutex
}

// NewEngine builds the engine over the library's files and its journal. now and log may
// be nil.
func NewEngine(files Files, journal Journal, now func() time.Time, log *slog.Logger) *Engine {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Engine{files: files, journal: journal, now: now, newID: randomName, log: log}
}

// randomName returns a random name for a folder or an operation: hex, never a path.
func randomName() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Builder plans a change. Its methods read the library and add steps; they change nothing.
type Builder struct {
	files Files
	opID  string
	steps []domain.Step
	// created and freed hold the paths (lowercased) that a planned step creates or moves away from.
	created, freed map[string]bool
	scratch        int
	record         func(Records) error
}

// Mkdir plans the creation of dir and of its missing parents, top first. A folder that
// exists, or is planned already, stays as it is.
func (b *Builder) Mkdir(dir string) error {
	var missing []string
	for d := dir; ; d = filepath.Dir(d) {
		ok, err := b.exists(d)
		if err != nil {
			return err
		}
		if ok {
			break
		}
		missing = append([]string{d}, missing...)
		if filepath.Dir(d) == d {
			break
		}
	}
	for _, d := range missing {
		b.add(domain.Step{Kind: domain.StepMkdir, To: d})
	}
	return nil
}

// Move plans the rename of from to to. The target must be free (ErrTargetTaken otherwise)
// and the source must exist. A move that only changes the case of a name goes through a
// scratch name first: on the case-insensitive shares the library lives on, the target
// "exists" as the source does.
func (b *Builder) Move(from, to string) error {
	if from == to {
		return nil
	}
	if ok, err := b.exists(from); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("catalog: %s is missing", from)
	}
	if strings.EqualFold(from, to) {
		if err := b.Mkdir(b.files.OpsPath(b.opID)); err != nil {
			return err
		}
		b.scratch++
		tmp := b.files.OpsPath(b.opID, fmt.Sprintf("case-%d", b.scratch))
		b.add(domain.Step{Kind: domain.StepRename, From: from, To: tmp})
		b.add(domain.Step{Kind: domain.StepRename, From: tmp, To: to})
		return nil
	}
	if ok, err := b.exists(to); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("%w: %s", ErrTargetTaken, to)
	}
	b.add(domain.Step{Kind: domain.StepRename, From: from, To: to})
	return nil
}

// Record sets the catalog records of the change, written in the commit (see Journal.Commit).
func (b *Builder) Record(fn func(Records) error) { b.record = fn }

func (b *Builder) add(s domain.Step) {
	switch s.Kind {
	case domain.StepMkdir:
		b.created[strings.ToLower(s.To)] = true
	case domain.StepRename:
		b.freed[strings.ToLower(s.From)] = true
		b.created[strings.ToLower(s.To)] = true
	}
	b.steps = append(b.steps, s)
}

// exists reports whether path is in the library or planned by this change.
func (b *Builder) exists(path string) (bool, error) {
	key := strings.ToLower(path)
	if b.created[key] {
		return true, nil
	}
	if b.freed[key] {
		return false, nil
	}
	return b.files.Exists(path)
}

// Do plans a change with build, performs it and records it. build runs under the engine's
// lock and reads the library. A change with no steps and no records does nothing.
func (e *Engine) Do(ctx context.Context, kind, source string, build func(*Builder) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	b := &Builder{files: e.files, opID: e.newID(), created: map[string]bool{}, freed: map[string]bool{}}
	if err := build(b); err != nil {
		return err
	}
	plan := domain.Plan{Kind: kind, Source: source, Steps: b.steps}
	if err := plan.Validate(); err != nil {
		return err
	}
	if len(plan.Steps) == 0 && b.record == nil {
		return nil
	}
	if err := e.checkTargets(plan); err != nil {
		return err
	}

	op := domain.Operation{ID: b.opID, Plan: plan, CreatedAt: e.now()}
	if err := e.journal.Save(ctx, op); err != nil {
		return fmt.Errorf("catalog: save the journal: %w", err)
	}
	for i, step := range plan.Steps {
		op.Started = i + 1
		if err := e.journal.Advance(ctx, op.ID, op.Started); err != nil {
			return e.fail(ctx, op, fmt.Errorf("catalog: record progress: %w", err))
		}
		if err := e.perform(step); err != nil {
			return e.fail(ctx, op, fmt.Errorf("catalog: %s: %w", step.Kind, err))
		}
	}

	record := b.record
	if record == nil {
		record = func(Records) error { return nil }
	}
	if err := e.journal.Commit(ctx, op.ID, record); err != nil {
		return e.fail(ctx, op, fmt.Errorf("catalog: record the change: %w", err))
	}
	e.removeScratch(op.ID)
	return nil
}

// checkTargets refuses a plan that would overwrite something the library holds now. A
// target that an earlier step of the plan moves away from is free.
func (e *Engine) checkTargets(plan domain.Plan) error {
	freed := map[string]bool{}
	for _, s := range plan.Steps {
		if s.Kind == domain.StepRename {
			freed[strings.ToLower(s.From)] = true
		}
		if s.Kind != domain.StepMkdir && s.Kind != domain.StepRename {
			continue
		}
		if freed[strings.ToLower(s.To)] {
			continue
		}
		taken, err := e.files.Exists(s.To)
		if err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("%w: %s", ErrTargetTaken, s.To)
		}
	}
	return nil
}

// perform runs one step of a plan.
func (e *Engine) perform(s domain.Step) error {
	switch s.Kind {
	case domain.StepMkdir:
		return e.files.Mkdir(s.To)
	case domain.StepRemoveDir:
		return e.files.RemoveDir(s.From)
	case domain.StepRename:
		return e.files.Move(s.From, s.To)
	default:
		return fmt.Errorf("%w: step %q", domain.ErrInvalidPlan, s.Kind)
	}
}

// fail undoes a change that failed and returns its cause. When the undo fails too, the
// journal entry stays and the error says so.
func (e *Engine) fail(ctx context.Context, op domain.Operation, cause error) error {
	if err := e.undo(context.WithoutCancel(ctx), op); err != nil {
		e.log.Error("library change not undone; it stays journaled", "operation", op.ID, "kind", op.Plan.Kind, "error", err)
		return fmt.Errorf("%w: %w", ErrUndoFailed, cause)
	}
	return cause
}

// undo reverts what an operation may have changed and forgets it. Each reverted step
// checks what is on disk first, so the undo is safe to repeat at any point.
func (e *Engine) undo(ctx context.Context, op domain.Operation) error {
	var errs []error
	for _, s := range op.Undo().Steps {
		if err := e.revert(s); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if err := e.journal.Forget(ctx, op.ID); err != nil {
		return fmt.Errorf("catalog: forget operation %s: %w", op.ID, err)
	}
	e.removeScratch(op.ID)
	return nil
}

// revert performs one inverse step (see domain.Operation.Undo) when the disk still shows
// the change it reverts. Nothing is deleted unless this change created it, except a
// partial copy this change left behind.
func (e *Engine) revert(s domain.Step) error {
	switch s.Kind {
	case domain.StepRename:
		// The file is at s.From and goes back to s.To.
		now, back := s.From, s.To
		nowOK, err := e.files.Exists(now)
		if err != nil {
			return err
		}
		backOK, err := e.files.Exists(back)
		if err != nil {
			return err
		}
		switch {
		case nowOK && backOK:
			return fmt.Errorf("%w: %s and %s", errBothExist, now, back)
		case nowOK:
			return e.files.Move(now, back)
		default:
			return e.files.RemoveCopy(now)
		}
	case domain.StepRemoveDir:
		ok, err := e.files.Exists(s.From)
		if err != nil || !ok {
			return err
		}
		err = e.files.RemoveDir(s.From)
		if errors.Is(err, ErrNotEmpty) {
			// Something was put in the folder after the change: it is not ours to delete.
			e.log.Warn("folder kept: it is not empty", "dir", s.From)
			return nil
		}
		return err
	case domain.StepMkdir:
		ok, err := e.files.Exists(s.To)
		if err != nil || ok {
			return err
		}
		return e.files.Mkdir(s.To)
	default:
		return fmt.Errorf("%w: step %q", domain.ErrInvalidPlan, s.Kind)
	}
}

// removeScratch deletes the scratch folder of an operation, if it has one, logging a failure.
func (e *Engine) removeScratch(id string) {
	if err := e.files.RemoveAll(e.files.OpsPath(id)); err != nil {
		e.log.Warn("scratch folder not removed", "operation", id, "error", err)
	}
}

// Recover undoes the operations that a restart interrupted and deletes the scratch names
// they left. It runs once at start, before anything else touches the library (RNF-04).
// It returns how many operations it found; an operation that cannot be undone stays in
// the journal and is reported.
func (e *Engine) Recover(ctx context.Context) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ops, err := e.journal.Pending(ctx)
	if err != nil {
		return 0, err
	}
	kept := map[string]bool{}
	var failed []error
	for _, op := range ops {
		e.log.Warn("undoing interrupted library change", "operation", op.ID, "kind", op.Plan.Kind, "started", op.Started, "steps", len(op.Plan.Steps))
		if err := e.undo(ctx, op); err != nil {
			kept[op.ID] = true
			failed = append(failed, fmt.Errorf("operation %s: %w", op.ID, err))
		}
	}
	names, err := e.files.Names(e.files.OpsPath())
	if err != nil {
		return len(ops), errors.Join(append(failed, err)...)
	}
	for _, n := range names {
		if !kept[n] {
			e.removeScratch(n)
		}
	}
	return len(ops), errors.Join(failed...)
}
