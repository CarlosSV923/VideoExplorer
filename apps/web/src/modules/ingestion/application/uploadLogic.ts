import type {
  JobFailure,
  JobStatus,
  LocalUpload,
  UploadJob,
  UploadPhase,
  UploadRow,
} from '../domain/types'

const multipartPatterns = [/^(.*)\.part(\d+)\.rar$/i, /^(.*)\.(7z|zip)\.(\d{3})$/i]

function multipartVolume(fileName: string): { group: string; number: number } | undefined {
  const baseName = fileName.split(/[\\/]/).at(-1) ?? fileName
  for (const pattern of multipartPatterns) {
    const match = pattern.exec(baseName)
    const number = match?.[3] ?? match?.[2]
    if (match?.[1] && number) return { group: match[1].toLowerCase(), number: Number(number) }
  }
  return undefined
}

/** Returns a stable group key for formats supported by the server. */
export function multipartGroup(fileName: string): string | undefined {
  return multipartVolume(fileName)?.group
}

/** Shows a set-level password prompt only on its first numbered volume. */
export function isFirstMultipartVolume(fileName: string, siblings: readonly string[]): boolean {
  const current = multipartVolume(fileName)
  if (!current) return true
  const first = siblings
    .map((name) => ({ name, volume: multipartVolume(name) }))
    .filter(
      (item): item is { name: string; volume: { group: string; number: number } } =>
        item.volume?.group === current.group,
    )
    .sort((a, b) => a.volume.number - b.volume.number || a.name.localeCompare(b.name))[0]
  return first?.name === fileName
}

/** Counts volumes in each multiparte set among the files selected by the user. */
export function multipartPartsTotal(
  files: readonly Pick<File, 'name'>[],
): ReadonlyMap<string, number> {
  const counts = new Map<string, number>()
  for (const file of files) {
    const group = multipartGroup(file.name)
    if (group) counts.set(group, (counts.get(group) ?? 0) + 1)
  }
  return counts
}

const cancelableStatuses: ReadonlySet<JobStatus> = new Set([
  'uploading',
  'uploaded',
  'waiting_parts',
  'needs_password',
  'extracting',
])

export function canCancelJob(status: JobStatus): boolean {
  return cancelableStatuses.has(status)
}

const retryableFailures: ReadonlySet<JobFailure> = new Set([
  'no_space',
  'archive_corrupt',
  'crc_mismatch',
  'filter_failed',
  'move_failed',
  'trash_failed',
])

export function canRetryJob(error: JobFailure | undefined): boolean {
  return error !== undefined && retryableFailures.has(error)
}

export const activeServerStatuses: ReadonlySet<JobStatus> = new Set([
  'uploading',
  'uploaded',
  'waiting_parts',
  'extracting',
  'needs_password',
  'filtering',
])

export function isJobActiveOrWaiting(job: Pick<UploadJob, 'status' | 'error'>): boolean {
  return activeServerStatuses.has(job.status) || (job.status === 'failed' && canRetryJob(job.error))
}

const serverPhases: Partial<Record<JobStatus, UploadPhase>> = {
  uploaded: 'uploaded',
  waiting_parts: 'waiting_parts',
  extracting: 'extracting',
  needs_password: 'needs_password',
  filtering: 'filtering',
  done: 'done',
  discarded: 'discarded',
  failed: 'failed',
  cancelled: 'cancelled',
}

/** Joins the browser queue with server jobs, including an upload interrupted by a reload. */
export function buildUploadRows(
  local: readonly LocalUpload[],
  jobs: readonly UploadJob[],
  now: number,
): UploadRow[] {
  const localByJob = new Map(
    local.flatMap((item) => (item.jobId ? [[item.jobId, item] as const] : [])),
  )
  const rows = jobs
    .filter((job) => job.status !== 'cancelled')
    .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
    .map<UploadRow>((job) => {
      const current = localByJob.get(job.id)
      let phase: UploadPhase = serverPhases[job.status] ?? 'failed'
      let sent = job.received
      if (job.status === 'uploading') {
        if (current?.state === 'queued') phase = 'queued'
        else if (current?.state === 'uploading') phase = 'uploading'
        else if (current?.state === 'error') phase = 'uploadError'
        else if (current?.state === 'finished') {
          phase = 'uploaded'
          sent = job.size
        } else if (now - Date.parse(job.updatedAt) >= 15_000) phase = 'interrupted'
      }
      return {
        key: job.id,
        jobId: job.id,
        ...(current ? { localKey: current.key } : {}),
        fileName: job.fileName,
        size: job.size,
        sent,
        phase,
        progress: job.progress,
        ...(job.error ? { error: job.error } : {}),
      }
    })
  const seen = new Set(rows.map((row) => row.jobId))
  const queued = local
    .filter((item) => !item.jobId || !seen.has(item.jobId))
    .map<UploadRow>((item) => ({
      key: item.key,
      localKey: item.key,
      ...(item.jobId ? { jobId: item.jobId } : {}),
      fileName: item.fileName,
      size: item.size,
      sent: item.sent,
      phase:
        item.state === 'queued'
          ? 'queued'
          : item.state === 'uploading'
            ? 'uploading'
            : 'uploadError',
    }))
  return [...queued, ...rows]
}
