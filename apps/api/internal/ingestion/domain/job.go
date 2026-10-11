// Package domain is the ingestion model: a batch groups the files dropped
// together (RF-03), and an UploadJob follows one uploaded file through the
// states of RF-10a. Everything here is pure: no I/O and no framework.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// JobID identifies an upload job. It is the tus upload id (RF-02): the browser
// resumes the upload with it and the server finds its job with the same value.
type JobID string

// Status is a state of an upload job (spec RF-10a).
type Status string

// The states of RF-10a. Only the upload ones (uploading, uploaded, failed,
// cancelled) are reached in phase 3; the rest are reached by the extraction
// and filtering of phase 4.
const (
	// StatusUploading: the bytes are arriving ("Subiendo").
	StatusUploading Status = "uploading"
	// StatusUploaded: every byte arrived ("Subido").
	StatusUploaded Status = "uploaded"
	// StatusWaitingParts: a volume of a multi-volume archive waits for the others ("Esperando partes").
	StatusWaitingParts Status = "waiting_parts"
	// StatusExtracting: an archive is being unpacked ("Descomprimiendo").
	StatusExtracting Status = "extracting"
	// StatusNeedsPassword: an encrypted archive waits for its password ("Necesita contraseña").
	StatusNeedsPassword Status = "needs_password"
	// StatusFiltering: the content is being filtered ("Filtrando").
	StatusFiltering Status = "filtering"
	// StatusDone: the content is in No asignados ("En No asignados").
	StatusDone Status = "done"
	// StatusFailed: stopped by an error, with its reason ("Error").
	StatusFailed Status = "failed"
	// StatusCancelled: stopped by the user or by the browser ("Cancelado").
	StatusCancelled Status = "cancelled"
)

// allStatuses lists every state of RF-10a, in the order of the spec.
var allStatuses = []Status{
	StatusUploading, StatusUploaded, StatusWaitingParts, StatusExtracting,
	StatusNeedsPassword, StatusFiltering, StatusDone, StatusFailed, StatusCancelled,
}

// transitions lists the states each state may move to. A state missing from the
// map is terminal. "Interrumpida" (RF-02) is not a state here: it is an upload
// that stops receiving bytes, which stays uploading.
var transitions = map[Status][]Status{
	StatusUploading:     {StatusUploaded, StatusFailed, StatusCancelled},
	StatusUploaded:      {StatusWaitingParts, StatusExtracting, StatusFiltering, StatusFailed, StatusCancelled},
	StatusWaitingParts:  {StatusExtracting, StatusFailed, StatusCancelled},
	StatusExtracting:    {StatusNeedsPassword, StatusFiltering, StatusFailed, StatusCancelled},
	StatusNeedsPassword: {StatusExtracting, StatusFailed, StatusCancelled},
	StatusFiltering:     {StatusDone, StatusFailed, StatusCancelled},
}

// Statuses returns every state of RF-10a.
func Statuses() []Status { return append([]Status(nil), allStatuses...) }

// Terminal reports whether no transition leaves s.
func (s Status) Terminal() bool {
	_, ok := transitions[s]
	return !ok
}

// CanTransitionTo reports whether the job may move from s to next.
func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// FailureReason is the stable code of why a job failed (RF-10b). The UI
// translates it with i18n keys, so the values never change once shipped.
type FailureReason string

// The reasons a job can fail with in phase 3 (RF-10b).
const (
	// FailureUploadAbandoned: the upload had no activity for AbandonedAfter; its partial file was deleted.
	FailureUploadAbandoned FailureReason = "upload_abandoned"
	// FailureUploadMissing: the job was still uploading, but its file is no longer in staging.
	FailureUploadMissing FailureReason = "upload_missing"
)

// Valid reports whether r is one of the reasons above.
func (r FailureReason) Valid() bool {
	switch r {
	case FailureUploadAbandoned, FailureUploadMissing:
		return true
	default:
		return false
	}
}

var (
	// ErrInvalidTransition is returned for a move the state machine forbids.
	ErrInvalidTransition = errors.New("invalid job state transition")
	// ErrJobNotFound is returned when no job has the id.
	ErrJobNotFound = errors.New("upload job not found")
	// ErrEmptyFile is returned for an upload of 0 bytes (RF-09a).
	ErrEmptyFile = errors.New("empty file")
)

// UploadJob is the state of one uploaded file (RF-10a). Several jobs of one
// batch form the batch's files (RF-03).
type UploadJob struct {
	ID JobID
	// BatchID is the batch the file was dropped with (RF-09a). Always set.
	BatchID BatchID
	// FileName is the name of the file, without folders.
	FileName string
	// RelativePath is the file's path inside the folder dropped in the browser
	// (RF-01a), ending with FileName; empty for a file on its own.
	RelativePath string
	// Size is the number of bytes the upload declared.
	Size int64
	// Received is how many of those bytes arrived so far (RF-10b). It never
	// goes back and never passes Size.
	Received int64
	Status   Status
	// Error is the stable reason of a failed job (RF-10b); empty otherwise.
	Error     FailureReason
	CreatedAt time.Time
	UpdatedAt time.Time
}

// JobView is a job with the batch it belongs to: what the API shows and the
// event stream sends (RF-03, RF-10b).
type JobView struct {
	Job   UploadJob
	Batch Batch
}

// NewUploadJob starts the job of an upload whose form data is already checked
// (UploadMeta) and whose size is declared. The job starts uploading.
func NewUploadJob(id JobID, meta UploadMeta, size int64, now time.Time) (*UploadJob, error) {
	if meta.BatchID == "" {
		return nil, fmt.Errorf("%w: an upload belongs to a batch", ErrInvalidMetadata)
	}
	if size == 0 {
		return nil, ErrEmptyFile
	}
	if size < 0 {
		return nil, fmt.Errorf("negative size %d", size)
	}
	return &UploadJob{
		ID:           id,
		BatchID:      BatchID(meta.BatchID),
		FileName:     meta.FileName,
		RelativePath: meta.RelativePath,
		Size:         size,
		Status:       StatusUploading,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// RecordProgress stores how many bytes arrived so far. Only an uploading job
// takes progress, the count never goes back, and it is capped at Size. It
// reports whether the job changed.
func (j *UploadJob) RecordProgress(received int64, now time.Time) bool {
	if j.Status != StatusUploading || received <= j.Received {
		return false
	}
	j.Received = min(received, j.Size)
	j.UpdatedAt = now
	return true
}

// ReconcileReceived raises the received count to the bytes the staged file holds,
// when that is more than recorded (capped at Size). It keeps UpdatedAt: the file
// does not tell when its bytes arrived, so reconciling must not reset the idle
// time of the upload (RF-10b). It reports whether the job changed.
func (j *UploadJob) ReconcileReceived(staged int64) bool {
	if j.Status != StatusUploading || staged <= j.Received {
		return false
	}
	j.Received = min(staged, j.Size)
	return true
}

// MarkUploaded records that every byte arrived (uploading → uploaded).
func (j *UploadJob) MarkUploaded(now time.Time) error {
	if err := j.moveTo(StatusUploaded, now); err != nil {
		return err
	}
	j.Received = j.Size
	return nil
}

// Fail stops the job with a stable reason (uploading → failed).
func (j *UploadJob) Fail(reason FailureReason, now time.Time) error {
	if !reason.Valid() {
		return fmt.Errorf("%w: unknown failure reason %q", ErrInvalidTransition, reason)
	}
	if err := j.moveTo(StatusFailed, now); err != nil {
		return err
	}
	j.Error = reason
	return nil
}

// Cancel stops the job at the user's or the browser's request (uploading or
// uploaded → cancelled).
func (j *UploadJob) Cancel(now time.Time) error {
	return j.moveTo(StatusCancelled, now)
}

// moveTo applies a transition of the table, or refuses it.
func (j *UploadJob) moveTo(next Status, now time.Time) error {
	if !j.Status.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, j.Status, next)
	}
	j.Status = next
	j.UpdatedAt = now
	return nil
}
