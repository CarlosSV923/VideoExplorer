import type { ApiClient } from '@/shared/api/client'
import { unwrap } from '@/shared/api/result'

import type { IngestionPorts } from '../../application/ports'

export function createIngestionHttp(client: ApiClient, baseUrl = '/api'): IngestionPorts {
  const job = (id: string) => ({ params: { path: { id } } })
  return {
    createBatch: (request) => unwrap(client.POST('/batches', { body: request })),
    jobs: () => unwrap(client.GET('/jobs')),
    watchJobs: ({ onJob, onOpen }) => {
      const source = new EventSource(`${baseUrl}/jobs/events`, { withCredentials: true })
      const listener = (event: MessageEvent<string>) => {
        onJob(JSON.parse(event.data) as Parameters<typeof onJob>[0])
      }
      source.addEventListener('job', listener)
      if (onOpen) source.addEventListener('open', onOpen)
      return () => {
        source.close()
      }
    },
    submitPassword: (id, password) =>
      unwrap(client.POST('/jobs/{id}/password', { ...job(id), body: { password } })),
    retryJob: (id) => unwrap(client.POST('/jobs/{id}/retry', job(id))),
    dismissJob: (id) => unwrap(client.POST('/jobs/{id}/dismiss', job(id))),
  }
}
