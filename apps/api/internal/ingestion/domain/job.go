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

// The states of RF-10a. The upload ones (uploading, uploaded) are reached in
// phase 3, the extraction ones (extracting, needs_password, filtering) in phase
// 4; waiting_parts and done come with the multi-volume and filtering steps.
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
	// StatusDiscarded: the filter discarded everything of the job while its batch keeps something ("Descartado", RF-07). Its content is in the trash.
	StatusDiscarded Status = "discarded"
	// StatusFailed: stopped by an error, with its reason ("Error").
	StatusFailed Status = "failed"
	// StatusCancelled: stopped by the user or by the browser ("Cancelado").
	StatusCancelled Status = "cancelled"
)

// allStatuses lists every state of RF-10a, in the order of the spec.
var allStatuses = []Status{
	StatusUploading, StatusUploaded, StatusWaitingParts, StatusExtracting,
	StatusNeedsPassword, StatusFiltering, StatusDone, StatusDiscarded, StatusFailed, StatusCancelled,
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
	StatusFiltering:     {StatusDone, StatusDiscarded, StatusFailed, StatusCancelled},
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

// FailureReason is the stable code of why a job failed or waits for a password
// (RF-10b, RF-10c). The UI translates it with i18n keys, so the values never
// change once shipped.
type FailureReason string

// The reasons of RF-10b and RF-10c.
const (
	// FailureUploadAbandoned: the upload had no activity for AbandonedAfter; its partial file was deleted.
	FailureUploadAbandoned FailureReason = "upload_abandoned"
	// FailureUploadMissing: the job was still uploading, but its file is no longer in staging.
	FailureUploadMissing FailureReason = "upload_missing"
	// FailureNoSpace: the library has no room to extract the archive plus the margin (RF-05).
	FailureNoSpace FailureReason = "no_space"
	// FailureArchiveCorrupt: the archive is damaged, or 7-Zip failed on it.
	FailureArchiveCorrupt FailureReason = "archive_corrupt"
	// FailureCRCMismatch: an extracted file differs in size or CRC32 from the archive index (RF-05).
	FailureCRCMismatch FailureReason = "crc_mismatch"
	// FailureArchiveUnsafe: an entry would leave its folder (zip-slip) or is a link.
	FailureArchiveUnsafe FailureReason = "archive_unsafe"
	// FailureWrongPassword: the password sent does not open the archive (the job stays in needs_password).
	FailureWrongPassword FailureReason = "wrong_password"
	// FailurePartsMismatch: the volumes of a multi-volume archive are not one set (RF-10c).
	FailurePartsMismatch FailureReason = "parts_mismatch"
	// FailurePartsMissing: some volumes did not arrive within AbandonedAfter (RF-10c).
	FailurePartsMissing FailureReason = "parts_missing"
	// FailureLegacyVolume: an old-style RAR volume (.r00, .r01…), which is not supported (RF-03).
	FailureLegacyVolume FailureReason = "legacy_volume"
	// FailureNoVideo: nothing is left to keep after filtering: no video and no subtitle (RF-07).
	FailureNoVideo FailureReason = "no_video"
	// FailureFilterFailed: reading or writing the staging area failed while filtering (RF-07, RF-08a).
	FailureFilterFailed FailureReason = "filter_failed"
	// FailureMoveFailed: reading or writing the library failed while moving the content to No asignados (RF-08a).
	FailureMoveFailed FailureReason = "move_failed"
)

// Valid reports whether r is one of the reasons above.
func (r FailureReason) Valid() bool {
	switch r {
	case FailureUploadAbandoned, FailureUploadMissing, FailureNoSpace, FailureArchiveCorrupt,
		FailureCRCMismatch, FailureArchiveUnsafe, FailureWrongPassword, FailurePartsMismatch,
		FailurePartsMissing, FailureLegacyVolume, FailureNoVideo, FailureFilterFailed, FailureMoveFailed:
		return true
	default:
		return false
	}
}

// Retryable reports whether a failed job may be retried (RF-10c, RF-08a): the reasons that
// a change outside the job can fix. RetriesExtraction says which of them go back to the
// extraction, and the others back to the filter.
func (r FailureReason) Retryable() bool {
	return r.RetriesExtraction() || r == FailureFilterFailed || r == FailureMoveFailed
}

// RetriesExtraction reports whether a failed extraction may be retried from its archive
// (RF-10c): only the reasons that a change outside the archive can fix.
func (r FailureReason) RetriesExtraction() bool {
	switch r {
	case FailureNoSpace, FailureArchiveCorrupt, FailureCRCMismatch:
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
	// Progress is the percentage extracted (0 to 100), while the archive is
	// being extracted (RF-10); 0 before and 100 once it is done.
	Progress int
	// PartsTotal, VolumeSet and VolumeIndex describe a volume of a multi-volume
	// archive (RF-03): how many volumes the set has, the set it belongs to (its
	// name, with the naming style) and its position, 1 for the first. They are
	// zero and empty for any other file.
	PartsTotal  int
	VolumeSet   string
	VolumeIndex int
	// VolumeGroup is the extraction of the set this volume is part of, once the
	// set is complete: the id of its first volume. Empty while the volume waits.
	VolumeGroup string
	// FilterDone marks a job whose content the filter has been through (RF-07): what
	// it keeps is in the filter's records and the job waits for the step that moves
	// it to No asignados. It is internal: the API does not show it.
	FilterDone bool
	// Error is the stable reason of a failed job (RF-10b, RF-10c), or of a job
	// in needs_password whose password was wrong; empty otherwise.
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
	job := &UploadJob{
		ID:           id,
		BatchID:      BatchID(meta.BatchID),
		FileName:     meta.FileName,
		RelativePath: meta.RelativePath,
		Size:         size,
		Status:       StatusUploading,
		PartsTotal:   meta.PartsTotal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if v, ok := ParseVolume(meta.FileName); ok {
		job.VolumeSet, job.VolumeIndex = v.Set, v.Index
	}
	return job, nil
}

// IsVolume reports whether the job is a volume of a multi-volume archive (RF-03).
func (j *UploadJob) IsVolume() bool { return j.VolumeSet != "" }

// WaitForParts parks a volume until the other volumes of its set arrive
// (uploaded → waiting_parts, RF-10a).
func (j *UploadJob) WaitForParts(now time.Time) error {
	return j.moveTo(StatusWaitingParts, now)
}

// ClaimSet takes a complete set for its extraction (waiting_parts → extracting).
// group names the set's extraction: the id of its first volume, which is then
// the job the extraction runs for (RF-03, RF-10a).
func (j *UploadJob) ClaimSet(group string, now time.Time) error {
	if !j.IsVolume() || group == "" {
		return fmt.Errorf("%w: only a volume of a set can be claimed", ErrInvalidTransition)
	}
	if err := j.moveTo(StatusExtracting, now); err != nil {
		return err
	}
	j.VolumeGroup, j.Error, j.Progress = group, "", 0
	return nil
}

// ResumeVolumeAfterRestart puts a volume whose set extraction a restart
// interrupted back to waiting_parts: the set is claimed again once it is
// complete (RF-10a, RF-10b). It reports whether the job changed.
func (j *UploadJob) ResumeVolumeAfterRestart(now time.Time) bool {
	if j.Status != StatusExtracting || !j.IsVolume() {
		return false
	}
	j.Status, j.Progress, j.VolumeGroup, j.UpdatedAt = StatusWaitingParts, 0, "", now
	return true
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

// StartExtraction begins extracting an uploaded archive, or extracts it again
// with the password a job waiting in needs_password was sent (uploaded or
// needs_password → extracting). It clears the reason and the progress.
func (j *UploadJob) StartExtraction(now time.Time) error {
	if err := j.moveTo(StatusExtracting, now); err != nil {
		return err
	}
	j.Error, j.Progress = "", 0
	return nil
}

// RecordExtractionProgress stores the percentage extracted. Only an extracting
// job takes it, the percentage only goes forward and it is capped at 100. It
// reports whether the job changed.
func (j *UploadJob) RecordExtractionProgress(pct int, now time.Time) bool {
	if j.Status != StatusExtracting || pct <= j.Progress {
		return false
	}
	j.Progress = min(pct, 100)
	j.UpdatedAt = now
	return true
}

// AskPassword pauses an extraction because the archive is encrypted and no
// password was given (extracting → needs_password, no reason).
func (j *UploadJob) AskPassword(now time.Time) error {
	return j.moveTo(StatusNeedsPassword, now)
}

// RejectPassword pauses an extraction because the password sent does not open
// the archive (extracting → needs_password, with wrong_password, RF-10c).
func (j *UploadJob) RejectPassword(now time.Time) error {
	if err := j.moveTo(StatusNeedsPassword, now); err != nil {
		return err
	}
	j.Error, j.Progress = FailureWrongPassword, 0
	return nil
}

// FinishExtraction records that the archive is extracted and its content waits
// for the filtering (extracting → filtering, RF-10a).
func (j *UploadJob) FinishExtraction(now time.Time) error {
	if err := j.moveTo(StatusFiltering, now); err != nil {
		return err
	}
	j.Progress = 100
	return nil
}

// RetryExtraction sends a failed extraction back to extracting (RF-10c). Only a
// failure with a retryable reason may be retried. The failed state is terminal
// in the transition table; this is the one way out, and it checks the reason.
func (j *UploadJob) RetryExtraction(now time.Time) error {
	if j.Status != StatusFailed || !j.Error.RetriesExtraction() {
		return fmt.Errorf("%w: %s (%q) cannot be retried", ErrInvalidTransition, j.Status, j.Error)
	}
	j.Status, j.Error, j.Progress, j.UpdatedAt = StatusExtracting, "", 0, now
	return nil
}

// RetryFiltering sends a job that failed while filtering or while moving its content back to
// filtering (filter_failed or move_failed, RF-07, RF-08a). The filter goes on from what it
// recorded; a job whose content was already filtered goes on to the move.
func (j *UploadJob) RetryFiltering(now time.Time) error {
	if j.Status != StatusFailed || (j.Error != FailureFilterFailed && j.Error != FailureMoveFailed) {
		return fmt.Errorf("%w: %s (%q) cannot be retried by filtering", ErrInvalidTransition, j.Status, j.Error)
	}
	j.Status, j.Error, j.UpdatedAt = StatusFiltering, "", now
	return nil
}

// Done records that the content of a filtered job is in No asignados (filtering → done,
// RF-08a). The job's files are then at their place in the library.
func (j *UploadJob) Done(now time.Time) error {
	return j.moveTo(StatusDone, now)
}

// ResumeAfterRestart puts an extraction that a restart interrupted back to
// uploaded, so it is extracted again from the archive (RF-10a, RF-10b). It
// reports whether the job changed.
func (j *UploadJob) ResumeAfterRestart(now time.Time) bool {
	if j.Status != StatusExtracting {
		return false
	}
	j.Status, j.Progress, j.UpdatedAt = StatusUploaded, 0, now
	return true
}

// StartFiltering moves an uploaded file that is not an archive to filtering: the
// file is kept as it was uploaded, and the filter takes it from here (RF-07, RF-10a).
func (j *UploadJob) StartFiltering(now time.Time) error {
	return j.moveTo(StatusFiltering, now)
}

// MarkFiltered records that the filter went through the content of the job (RF-07).
// The job stays filtering: moving the kept files to No asignados is a later step.
func (j *UploadJob) MarkFiltered(now time.Time) error {
	if j.Status != StatusFiltering {
		return fmt.Errorf("%w: only a job being filtered is marked as filtered, not %s", ErrInvalidTransition, j.Status)
	}
	j.FilterDone, j.UpdatedAt = true, now
	return nil
}

// Discard ends a job the filter emptied while its batch keeps something (filtering →
// discarded, RF-07). Its files are in the trash.
func (j *UploadJob) Discard(now time.Time) error {
	return j.moveTo(StatusDiscarded, now)
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
