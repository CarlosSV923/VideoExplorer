import type { components } from '@/shared/api/schema'

export type BatchRelation = components['schemas']['BatchRelation']
export type CreateBatchRequest = components['schemas']['CreateBatchRequest']
export type Batch = components['schemas']['Batch']
export type UploadJob = components['schemas']['UploadJob']
export type JobFailure = components['schemas']['JobFailure']
export type JobStatus = components['schemas']['JobStatus']
export type DownloadUrl = string

export type LocalUploadState = 'queued' | 'uploading' | 'error' | 'finished'

export interface LocalUpload {
  key: string
  fileName: string
  size: number
  sent: number
  state: LocalUploadState
  jobId?: string
  createdAt: number
}

export type UploadPhase =
  | 'queued'
  | 'uploading'
  | 'interrupted'
  | 'uploadError'
  | 'uploaded'
  | 'waiting_parts'
  | 'extracting'
  | 'needs_password'
  | 'filtering'
  | 'done'
  | 'discarded'
  | 'failed'
  | 'cancelled'

export interface UploadRow {
  key: string
  jobId?: string
  localKey?: string
  fileName: string
  size: number
  sent: number
  phase: UploadPhase
  progress?: number
  error?: JobFailure
}
