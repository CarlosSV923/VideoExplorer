-- +goose Up
-- Phase 5 (RF-07, RF-08a): deciding a batch first asks whether any of its jobs is still on
-- its way. This index answers it by seeks on the batch and its status, not by reading the
-- whole batch, so that the settlement of a batch costs the same whatever its size.
CREATE INDEX upload_jobs_batch_status ON upload_jobs (batch_id, status, filter_done);

-- +goose Down
DROP INDEX upload_jobs_batch_status;
