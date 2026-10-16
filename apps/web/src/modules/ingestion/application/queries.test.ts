import { QueryClient } from '@tanstack/react-query'

import { applyJobEvent, jobKey, jobsKey, watchJobsInCache } from './queries'
import type { IngestionPorts } from './ports'
import type { UploadJob } from '../domain/types'

const job = (updatedAt: string, status: UploadJob['status'] = 'uploading'): UploadJob => ({
  id: 'job-1',
  batch: {
    id: 'batch-1',
    name: 'Batch',
    relation: 'none',
    createdAt: '2026-01-01T00:00:00Z',
  },
  fileName: 'movie.mkv',
  relativePath: '',
  size: 100,
  received: status === 'done' ? 100 : 50,
  progress: status === 'done' ? 100 : 0,
  status,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt,
})

describe('job event cache updates', () => {
  it('keeps the most recent event when SSE messages arrive out of order', () => {
    const client = new QueryClient()
    const newer = job('2026-01-01T00:00:02Z', 'done')
    const older = job('2026-01-01T00:00:01Z')

    applyJobEvent(client, newer)
    applyJobEvent(client, older)

    expect(client.getQueryData<UploadJob[]>(jobsKey)?.[0]?.status).toBe('done')
  })

  it('notifies catalog invalidation only when a job changes status', () => {
    const client = new QueryClient()
    const previous = job('2026-01-01T00:00:01Z', 'extracting')
    const onStatusChange = vi.fn()
    let onJob: ((next: UploadJob) => void) | undefined
    const ports = {
      watchJobs: vi.fn((handlers: { onJob: (next: UploadJob) => void }) => {
        onJob = handlers.onJob
        return () => undefined
      }),
    } as unknown as IngestionPorts

    client.setQueryData(jobKey(previous.id), previous)
    watchJobsInCache(client, ports, { onStatusChange })
    onJob?.(job('2026-01-01T00:00:02Z', 'extracting'))
    onJob?.(job('2026-01-01T00:00:03Z', 'done'))

    expect(onStatusChange).toHaveBeenCalledTimes(1)
    expect(onStatusChange).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'done' }),
      expect.objectContaining({ status: 'extracting' }),
    )
  })
})
