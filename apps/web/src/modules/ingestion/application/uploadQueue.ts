import type { IngestionPorts, UploadHandle, UploadSpec } from './ports'
import type { LocalUpload, UploadJob } from '../domain/types'

interface Entry extends LocalUpload {
  file: File | null
  spec: UploadSpec
  handle?: UploadHandle
}

/** Browser-owned queue: only two tus transfers run at once. */
export class UploadQueue {
  private entries: Entry[] = []
  private snapshot: readonly LocalUpload[] = []
  private listeners = new Set<() => void>()
  private sequence = 0

  constructor(
    private readonly ports: Pick<IngestionPorts, 'upload' | 'cancelUpload'>,
    private readonly limit = 2,
  ) {}

  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  getSnapshot = () => this.snapshot

  add(items: readonly { file: File; spec: UploadSpec }[]) {
    for (const item of items) {
      this.entries.push({
        key: `local-${String(++this.sequence)}`,
        fileName: item.file.name,
        size: item.file.size,
        sent: 0,
        state: 'queued',
        createdAt: Date.now(),
        file: item.file,
        spec: item.spec,
      })
    }
    this.pump()
  }

  resume(job: Pick<UploadJob, 'id' | 'fileName' | 'size' | 'batch' | 'relativePath'>, file: File) {
    if (file.name !== job.fileName || file.size !== job.size) return false
    this.entries = this.entries.filter((entry) => entry.jobId !== job.id)
    this.entries.push({
      key: `local-${String(++this.sequence)}`,
      fileName: file.name,
      size: file.size,
      sent: 0,
      state: 'queued',
      jobId: job.id,
      createdAt: Date.now(),
      file,
      spec: {
        batchId: job.batch.id,
        relation: job.batch.relation,
        fileName: file.name,
        ...(job.relativePath ? { relativePath: job.relativePath } : {}),
        resumeJobId: job.id,
      },
    })
    this.pump()
    return true
  }

  retry(key: string) {
    const entry = this.entries.find((item) => item.key === key)
    if (!entry) return
    entry.state = 'queued'
    this.emit()
    this.pump()
  }

  async cancel(row: { key: string; jobId?: string }) {
    const entry = this.entries.find((item) => item.key === row.key || item.jobId === row.jobId)
    if (entry) {
      entry.handle?.abort()
      this.entries = this.entries.filter((item) => item !== entry)
      this.emit()
      this.pump()
    }
    if (row.jobId) await this.ports.cancelUpload(row.jobId)
  }

  busy() {
    return this.entries.some((entry) => entry.state === 'queued' || entry.state === 'uploading')
  }

  private pump() {
    let running = this.entries.filter((entry) => entry.state === 'uploading').length
    for (const entry of this.entries) {
      if (running >= this.limit) break
      if (entry.state !== 'queued' || !entry.file || !this.ports.upload) continue
      running++
      this.start(entry, entry.file)
    }
    this.emit()
  }

  private start(entry: Entry, file: File) {
    entry.state = 'uploading'
    const key = entry.key
    const upload = this.ports.upload
    if (!upload) return
    entry.handle = upload(file, entry.spec, {
      onJobId: (jobId) => {
        this.update(key, { jobId })
      },
      onProgress: (sent) => {
        this.update(key, { sent })
      },
      onSuccess: () => {
        this.update(key, { state: 'finished', sent: file.size })
        const done = this.entries.find((item) => item.key === key)
        if (done) {
          done.file = null
          delete done.handle
        }
        this.pump()
      },
      onError: () => {
        this.update(key, { state: 'error' })
        this.pump()
      },
    })
  }

  private update(key: string, patch: Partial<LocalUpload>) {
    const entry = this.entries.find((item) => item.key === key)
    if (!entry) return
    Object.assign(entry, patch)
    this.emit()
  }

  private emit() {
    this.snapshot = this.entries.map(({ key, fileName, size, sent, state, jobId, createdAt }) => ({
      key,
      fileName,
      size,
      sent,
      state,
      createdAt,
      ...(jobId ? { jobId } : {}),
    }))
    for (const listener of this.listeners) listener()
  }
}
