package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// BatchID identifies a batch. The server makes it, so it is always URL-safe and
// the browser only echoes it back as batchId (spec RF-03, RF-09a).
type BatchID string

// Relation says how the files dropped together relate (spec RF-03).
type Relation string

// Relations a batch can declare.
const (
	// RelationParts: parts of one film or series (cd1/cd2, several files of one work).
	RelationParts Relation = "parts"
	// RelationEpisodes: chapters of one series.
	RelationEpisodes Relation = "episodes"
	// RelationNone: no relation between the files (and the only one a single file gets).
	RelationNone Relation = "none"
)

// Valid reports whether r is one of the relations of RF-03.
func (r Relation) Valid() bool {
	switch r {
	case RelationParts, RelationEpisodes, RelationNone:
		return true
	default:
		return false
	}
}

var (
	// ErrInvalidBatch means the values declared for a batch are not usable.
	ErrInvalidBatch = errors.New("invalid batch")
	// ErrBatchNotFound means no batch has the id.
	ErrBatchNotFound = errors.New("batch not found")
)

// Batch is a set of files dropped together in the browser (spec RF-03). Its
// relation is the answer the user gave once, for all the files of the set.
type Batch struct {
	ID BatchID
	// Name is provisional: what the browser proposed, such as a dropped folder's
	// name (RF-01a). The final name comes at the end of processing (RF-08).
	Name     string
	Relation Relation
	// CreatedAt is when the browser asked for the batch.
	CreatedAt time.Time
}

// NewBatch checks the values a browser declares for a new batch and returns the
// batch. An empty name is allowed; a name is trimmed and must be usable as one
// path component (no separators, no control characters, at most 255 bytes).
func NewBatch(id BatchID, name string, relation Relation, now time.Time) (*Batch, error) {
	if !batchIDPattern.MatchString(string(id)) {
		return nil, fmt.Errorf("%w: id %q", ErrInvalidBatch, id)
	}
	if !relation.Valid() {
		return nil, fmt.Errorf("%w: relation %q must be parts, episodes or none", ErrInvalidBatch, relation)
	}
	name = strings.TrimSpace(name)
	if name != "" {
		clean, err := cleanComponent(name)
		if err != nil {
			return nil, fmt.Errorf("%w: name %w", ErrInvalidBatch, err)
		}
		name = clean
	}
	return &Batch{ID: id, Name: name, Relation: relation, CreatedAt: now}, nil
}
