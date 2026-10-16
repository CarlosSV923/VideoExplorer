import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'

import { useIngestionPorts, type IngestionPorts } from './ports'
import type { UploadJob } from '../domain/types'

export const jobsKey = ['jobs'] as const
export const jobKey = (id: string) => ['jobs', id] as const

function isNewer(next: UploadJob, current: UploadJob): boolean {
  return Date.parse(next.updatedAt) >= Date.parse(current.updatedAt)
}

/** Applies an SSE snapshot/event only when it is at least as recent as cache. */
export function applyJobEvent(queryClient: QueryClient, next: UploadJob): void {
  queryClient.setQueryData<UploadJob>(jobKey(next.id), (current) =>
    !current || isNewer(next, current) ? next : current,
  )
  queryClient.setQueryData<UploadJob[]>(jobsKey, (current) => {
    if (!current) return [next]
    const index = current.findIndex((job) => job.id === next.id)
    if (index < 0) return [next, ...current].slice(0, 100)
    const old = current[index]
    if (!old || !isNewer(next, old)) return current
    const updated = [...current]
    updated[index] = next
    return updated
  })
}

export function watchJobsInCache(queryClient: QueryClient, ports: IngestionPorts): () => void {
  return ports.watchJobs({
    onJob: (job) => {
      applyJobEvent(queryClient, job)
    },
  })
}

export function useJobs() {
  const ports = useIngestionPorts()
  return useQuery({ queryKey: jobsKey, queryFn: () => ports.jobs() })
}

export function useJobMutation(
  action: 'submitPassword' | 'retryJob' | 'dismissJob',
  jobId: string,
) {
  const ports = useIngestionPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (value?: string) => {
      if (action === 'submitPassword') return ports.submitPassword(jobId, value ?? '')
      if (action === 'retryJob') return ports.retryJob(jobId)
      return ports.dismissJob(jobId)
    },
    onSuccess: (job) => {
      applyJobEvent(queryClient, job)
    },
  })
}
