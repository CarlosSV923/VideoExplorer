package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

// Journal implements application.Journal on the shared database (RNF-04).
type Journal struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

var _ application.Journal = (*Journal)(nil)

// NewJournal builds the journal over an open database.
func NewJournal(db *sql.DB) *Journal {
	return &Journal{db: db, q: sqlcgen.New(db)}
}

// Save implements application.Journal.
func (j *Journal) Save(ctx context.Context, op domain.Operation) error {
	steps, err := json.Marshal(nonNilSteps(op.Plan.Steps))
	if err != nil {
		return fmt.Errorf("catalog: encode steps: %w", err)
	}
	return j.q.InsertOperation(ctx, sqlcgen.InsertOperationParams{
		ID: op.ID, Kind: op.Plan.Kind, Source: op.Plan.Source, Steps: string(steps),
		Started: int64(op.Started), CreatedAt: formatTime(op.CreatedAt),
	})
}

// Advance implements application.Journal.
func (j *Journal) Advance(ctx context.Context, id string, started int) error {
	return j.q.AdvanceOperation(ctx, sqlcgen.AdvanceOperationParams{Started: int64(started), ID: id})
}

// Pending implements application.Journal.
func (j *Journal) Pending(ctx context.Context) ([]domain.Operation, error) {
	rows, err := j.q.ListOperations(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list operations: %w", err)
	}
	out := make([]domain.Operation, 0, len(rows))
	for _, row := range rows {
		op := domain.Operation{ID: row.ID, Plan: domain.Plan{Kind: row.Kind, Source: row.Source}, Started: int(row.Started)}
		if err := json.Unmarshal([]byte(row.Steps), &op.Plan.Steps); err != nil {
			return nil, fmt.Errorf("catalog: operation %s: decode steps: %w", row.ID, err)
		}
		if op.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt); err != nil {
			return nil, fmt.Errorf("catalog: operation %s: %w", row.ID, err)
		}
		out = append(out, op)
	}
	return out, nil
}

// Forget implements application.Journal.
func (j *Journal) Forget(ctx context.Context, id string) error {
	return j.q.DeleteOperation(ctx, id)
}

// Commit implements application.Journal: record and the deletion of the operation share one
// transaction, so a failure of either leaves the operation in the journal.
func (j *Journal) Commit(ctx context.Context, id string, record func(application.Records) error) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: begin commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := j.q.WithTx(tx)
	if err := record(records{q: q}); err != nil {
		return err
	}
	if err := q.DeleteOperation(ctx, id); err != nil {
		return fmt.Errorf("catalog: delete operation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("catalog: commit: %w", err)
	}
	return nil
}

// nonNilSteps keeps an empty plan encoded as an array, not null.
func nonNilSteps(steps []domain.Step) []domain.Step {
	if steps == nil {
		return []domain.Step{}
	}
	return steps
}
