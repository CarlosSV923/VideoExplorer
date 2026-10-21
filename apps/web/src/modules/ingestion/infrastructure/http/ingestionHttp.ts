import { DetailedError, Upload } from 'tus-js-client'

import type { ApiClient } from '@/shared/api/client'
import { problemError, unwrap } from '@/shared/api/result'
import { AppError } from '@/shared/kernel/errors'

import type { IngestionPorts } from '../../application/ports'

export function createIngestionHttp(client: ApiClient, baseUrl = '/api'): IngestionPorts {
  const job = (id: string) => ({ params: { path: { id } } })
  return {
    createBatch: (request) => unwrap(client.POST('/batches', { body: request })),
    upload: (file, spec, callbacks) => {
      const upload = new Upload(file, {
        endpoint: `${baseUrl}/uploads/`,
        ...(spec.resumeJobId ? { uploadUrl: `${baseUrl}/uploads/${spec.resumeJobId}` } : {}),
        metadata: {
          filename: spec.fileName,
          ...(spec.relativePath ? { relativePath: spec.relativePath } : {}),
          batchId: spec.batchId,
          ...(spec.partsTotal === undefined ? {} : { partsTotal: String(spec.partsTotal) }),
        },
        retryDelays: [0, 1000, 3000, 5000, 10_000, 20_000],
        storeFingerprintForResuming: false,
        onUploadUrlAvailable: () => {
          const url = upload.url
          if (url) callbacks.onJobId(url.slice(url.lastIndexOf('/') + 1))
        },
        onProgress: callbacks.onProgress,
        onSuccess: callbacks.onSuccess,
        onError: (error) => {
          if (error instanceof DetailedError && error.originalResponse) {
            callbacks.onError(problemError(error.originalResponse.getStatus(), undefined))
          } else {
            callbacks.onError(new AppError('network', { detail: error.message }))
          }
        },
      })
      upload.start()
      return { abort: () => void upload.abort(false) }
    },
    jobs: () => unwrap(client.GET('/jobs')),
    watchJobs: ({ onJob, onOpen, onCatalogChanged }) => {
      const source = new EventSource(`${baseUrl}/jobs/events`, { withCredentials: true })
      const listener = (event: MessageEvent<string>) => {
        onJob(JSON.parse(event.data) as Parameters<typeof onJob>[0])
      }
      source.addEventListener('job', listener)
      if (onCatalogChanged) source.addEventListener('catalog', onCatalogChanged)
      if (onOpen) source.addEventListener('open', onOpen)
      return () => {
        source.close()
      }
    },
    submitPassword: (id, password) =>
      unwrap(client.POST('/jobs/{id}/password', { ...job(id), body: { password } })),
    retryJob: (id) => unwrap(client.POST('/jobs/{id}/retry', job(id))),
    dismissJob: (id) => unwrap(client.POST('/jobs/{id}/dismiss', job(id))),
    cancelUpload: async (id) => {
      const response = await fetch(`${baseUrl}/uploads/${encodeURIComponent(id)}`, {
        method: 'DELETE',
        credentials: 'same-origin',
      })
      if (!response.ok) throw problemError(response.status, undefined)
    },
  }
}
