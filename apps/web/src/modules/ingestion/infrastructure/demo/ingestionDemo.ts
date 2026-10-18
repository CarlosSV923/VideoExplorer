/* eslint @typescript-eslint/require-await: off */
import type { DemoCatalogAdapter } from '@/modules/catalog/infrastructure/demo/catalogDemo'
import type { IngestionPorts } from '@/modules/ingestion/application/ports'
import type { Batch, CreateBatchRequest, UploadJob } from '@/modules/ingestion/domain/types'

export function createIngestionDemo(catalogPorts: DemoCatalogAdapter): IngestionPorts {
  let sequence = 0
  const batches = new Map<string, Batch>()
  const jobs = new Map<string, UploadJob>()
  const listeners = new Set<(job: UploadJob) => void>()
  const finishedBatches = new Set<string>()
  const now = () => new Date().toISOString()
  const publish = (job: UploadJob) => {
    job.updatedAt = now()
    for (const listener of listeners) listener({ ...job, batch: { ...job.batch } })
  }
  const makeBatch = (request: CreateBatchRequest): Batch => {
    const batch = {
      id: `demo-batch-${String(++sequence)}`,
      name: request.name ?? '',
      relation: request.relation,
      createdAt: now(),
    }
    batches.set(batch.id, batch)
    return batch
  }
  const complete = (job: UploadJob) => {
    const archive = /\.(rar|zip|7z)(\.\d+)?$/i.test(job.fileName)
    const multipart = /\.part\d+\.(rar|7z)$/i.test(job.fileName)
    if (multipart && job.batch.relation === 'parts') {
      job.status = 'waiting_parts'
      publish(job)
      const related = [...jobs.values()].filter((other) => other.batch.id === job.batch.id)
      if (
        related.some((other) => other.status === 'waiting_parts') &&
        related.every((other) => other.received === other.size)
      ) {
        const first = related.sort((a, b) => a.fileName.localeCompare(b.fileName))[0]
        if (first) {
          first.status = first.batch.name.startsWith('sample:') ? 'needs_password' : 'extracting'
          first.progress = 0
          publish(first)
          for (const other of related.slice(1)) {
            other.status = 'done'
            publish(other)
          }
          if (first.status === 'extracting') advance(first)
        }
      }
      return
    }
    if (/sample/i.test(job.relativePath)) {
      job.status = 'discarded'
      publish(job)
      catalogPorts.addFilteredExample(job.relativePath, job.size)
      return
    }
    if (archive && job.batch.name.startsWith('sample:')) {
      job.status = 'needs_password'
      publish(job)
      return
    }
    if (archive) {
      job.status = 'extracting'
      job.progress = 0
      publish(job)
      advance(job)
      return
    }
    job.status = 'filtering'
    job.progress = 0
    publish(job)
    window.setTimeout(() => {
      job.status = 'done'
      job.progress = 100
      publish(job)
      if (finishedBatches.has(job.batch.id)) return
      const group = [...jobs.values()].filter((item) => item.batch.id === job.batch.id)
      if (!group.every((item) => item.status === 'done')) return
      finishedBatches.add(job.batch.id)
      const paths = group.map((item) => item.fileName)
      const isArchive = paths.some((path) => /\.(rar|zip|7z)(\.\d+)?$/i.test(path))
      const firstName = paths[0] ?? job.fileName
      const outputName = isArchive
        ? firstName.replace(/\.part\d+\.(rar|7z)$/i, '').replace(/\.(rar|zip|7z)(\.\d+)?$/i, '')
        : firstName.replace(/\.[^.]+$/, '')
      catalogPorts.addUploadEntry(
        outputName,
        [outputName + '.mkv'],
        group.reduce((sum, item) => sum + item.size, 0),
      )
    }, 350)
  }
  const advance = (job: UploadJob) => {
    let progress = 0
    const tick = () => {
      if (job.status !== 'extracting') return
      progress = Math.min(progress + 25, 100)
      job.progress = progress
      publish(job)
      if (progress < 100) window.setTimeout(tick, 100)
      else {
        job.status = 'filtering'
        publish(job)
        window.setTimeout(() => {
          job.status = 'done'
          job.progress = 100
          publish(job)
          const output = job.fileName
            .replace(/\.part\d+\.(rar|7z)$/i, '')
            .replace(/\.(rar|zip|7z)(\.\d+)?$/i, '')
          catalogPorts.addUploadEntry(output, [output + '.mkv'], job.size)
          catalogPorts.addConsumedArchive(job.fileName, job.size)
        }, 150)
      }
    }
    window.setTimeout(tick, 100)
  }
  return {
    createBatch: async (request) => makeBatch(request),
    upload: (file, spec, callbacks) => {
      const batch = batches.get(spec.batchId) ?? makeBatch({ relation: spec.relation })
      const id = `demo-job-${String(++sequence)}`
      const job: UploadJob = {
        id,
        batch,
        fileName: spec.fileName,
        relativePath: spec.relativePath ?? '',
        size: file.size,
        received: 0,
        progress: 0,
        status: 'uploading',
        createdAt: now(),
        updatedAt: now(),
      }
      jobs.set(id, job)
      callbacks.onJobId(id)
      publish(job)
      let received = 0
      const timer = window.setInterval(() => {
        received = Math.min(file.size, received + Math.max(1, Math.ceil(file.size / 5)))
        job.received = received
        callbacks.onProgress(received, file.size)
        publish(job)
        if (received >= file.size) {
          window.clearInterval(timer)
          job.status = 'uploaded'
          publish(job)
          callbacks.onSuccess()
          window.setTimeout(() => {
            complete(job)
          }, 120)
        }
      }, 90)
      return {
        abort: () => {
          window.clearInterval(timer)
        },
      }
    },
    jobs: async () => [...jobs.values()].map((job) => ({ ...job })),
    watchJobs: ({ onJob, onOpen }) => {
      listeners.add(onJob)
      onOpen?.()
      return () => {
        listeners.delete(onJob)
      }
    },
    submitPassword: async (id, password) => {
      const job = jobs.get(id)
      if (!job) throw new Error('Missing job')
      if (password !== 'demo') {
        job.error = 'wrong_password'
        publish(job)
        return job
      }
      delete job.error
      job.status = 'extracting'
      job.progress = 0
      publish(job)
      advance(job)
      return job
    },
    retryJob: async (id) => {
      const job = jobs.get(id)
      if (!job) throw new Error('Missing job')
      return job
    },
    dismissJob: async (id) => {
      const job = jobs.get(id)
      if (!job) throw new Error('Missing job')
      return job
    },
    cancelUpload: async (id) => {
      const job = jobs.get(id)
      if (job) {
        job.status = 'cancelled'
        publish(job)
      }
    },
  }
}
