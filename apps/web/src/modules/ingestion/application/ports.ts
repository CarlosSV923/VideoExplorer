import { createPortContext } from '@/shared/kernel/ports'

import type { AppError } from '@/shared/kernel/errors'

import type { Batch, BatchRelation, CreateBatchRequest, UploadJob } from '../domain/types'

export interface UploadSpec {
  batchId: string
  relation: BatchRelation
  fileName: string
  relativePath?: string
  partsTotal?: number
}

export interface UploadCallbacks {
  onJobId: (jobId: string) => void
  onProgress: (received: number, total: number) => void
  onSuccess: () => void
  onError: (error: AppError) => void
}

export interface UploadHandle {
  abort(): void
}

export interface IngestionPorts {
  createBatch(request: CreateBatchRequest): Promise<Batch>
  /** Resumable tus upload; implemented by the HTTP adapter in the next task. */
  upload?: (file: File, spec: UploadSpec, callbacks: UploadCallbacks) => UploadHandle
  jobs(): Promise<UploadJob[]>
  watchJobs(handlers: { onJob: (job: UploadJob) => void; onOpen?: () => void }): () => void
  submitPassword(jobId: string, password: string): Promise<UploadJob>
  retryJob(jobId: string): Promise<UploadJob>
  dismissJob(jobId: string): Promise<UploadJob>
}

export const [IngestionContext, useIngestionPorts] = createPortContext<IngestionPorts>('Ingestion')
