import { useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useIngestionPorts } from '../application/ports'
import { useJobs, useJobMutation, useUploadQueue } from '../application/queries'
import { UploadQueue } from '../application/uploadQueue'
import {
  multipartGroup,
  multipartPartsTotal,
  isFirstMultipartVolume,
  buildUploadRows,
  canCancelJob,
  canRetryJob,
} from '../application/uploadLogic'
import { UploadQueueContext } from '../application/queries'
import type { UploadJob, UploadRow } from '../domain/types'
import { UploadUiContext, useUploadUi } from './UploadUiContext'
import { useFormat } from '@/shared/i18n/hooks'
import {
  Button,
  Dialog,
  EmptyState,
  FileIcon,
  PasswordField,
  ProgressBar,
  SidePanel,
  Sheet,
  StatusChip,
  UploadIcon,
} from '@/shared/ui'

type DroppedFile = File & { webkitRelativePath?: string }

function fileGroupName(files: readonly File[]): string | undefined {
  const relative = (files[0] as DroppedFile | undefined)?.webkitRelativePath
  if (relative?.includes('/')) return relative.split('/')[0]
  const first = files[0]?.name
  return first?.replace(/\.[^.]+$/, '')
}

function relationLabelKey(relation: 'parts' | 'episodes' | 'none') {
  return `uploads.relation.${relation}` as const
}

async function filesFromDrop(dataTransfer: DataTransfer): Promise<File[]> {
  const entries = Array.from(dataTransfer.items)
    .map((item) =>
      (
        item as DataTransferItem & { webkitGetAsEntry: () => FileSystemEntry | null }
      ).webkitGetAsEntry(),
    )
    .filter((entry): entry is FileSystemEntry => Boolean(entry))
  if (!entries.length) return Array.from(dataTransfer.files)

  const walk = async (entry: FileSystemEntry, prefix = ''): Promise<File[]> => {
    if (entry.isFile) {
      return new Promise((resolve) => {
        ;(entry as FileSystemFileEntry).file((file) => {
          Object.defineProperty(file, 'webkitRelativePath', {
            configurable: true,
            value: `${prefix}${file.name}`,
          })
          resolve([file])
        })
      })
    }
    const reader = (entry as FileSystemDirectoryEntry).createReader()
    const children: FileSystemEntry[] = []
    const read = () =>
      new Promise<void>((resolve, reject) => {
        reader.readEntries((batch) => {
          if (!batch.length) resolve()
          else {
            children.push(...batch)
            void read().then(resolve).catch(reject)
          }
        }, reject)
      })
    await read()
    const nested = await Promise.all(
      children.map((child) => walk(child, `${prefix}${entry.name}/`)),
    )
    return nested.flat()
  }
  return (await Promise.all(entries.map((entry) => walk(entry)))).flat()
}

function UploadRelationDialog({
  files,
  onChoose,
  onClose,
}: {
  files: File[]
  onChoose: (relation: 'parts' | 'episodes' | 'none') => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog title={t('uploads.relation.title')} onClose={onClose}>
      <p className="m-0 text-body-sm text-ink-2">
        {t('uploads.relation.body', { count: files.length })}
      </p>
      <div className="grid gap-3 sm:grid-cols-3">
        {(['parts', 'episodes', 'none'] as const).map((relation) => (
          <Button
            key={relation}
            variant="secondary"
            onClick={() => {
              onChoose(relation)
            }}
          >
            {t(relationLabelKey(relation))}
          </Button>
        ))}
      </div>
    </Dialog>
  )
}

function UploadRowCard({
  row,
  job,
  showPassword,
  onResume,
  onOpenPanel,
}: {
  row: UploadRow
  job?: UploadJob
  showPassword: boolean
  onResume: (job: UploadJob, file: File) => boolean
  onOpenPanel: () => void
}) {
  const { t } = useTranslation()
  const format = useFormat()
  const queue = useUploadQueue()
  const retry = useJobMutation('retryJob', job?.id ?? '')
  const password = useJobMutation('submitPassword', job?.id ?? '')
  const [secret, setSecret] = useState('')
  const [resumeError, setResumeError] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const hasUploadProgress =
    row.phase === 'queued' ||
    row.phase === 'uploading' ||
    row.phase === 'uploadError' ||
    row.phase === 'interrupted' ||
    row.phase === 'uploaded'
  const value = hasUploadProgress
    ? row.size
      ? (row.sent / row.size) * 100
      : 0
    : (row.progress ?? 0)
  const status = t(`uploads.status.${row.phase}` as never)

  return (
    <article className="flex flex-col gap-3 rounded-lg border border-line bg-surface-2 p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <FileIcon className="mt-0.5 shrink-0 text-ink-2" />
          <div className="min-w-0">
            <p className="m-0 break-all font-mono text-body-sm">{row.fileName}</p>
            <p className="m-0 mt-1 text-caption text-ink-2">{format.size(row.size)}</p>
          </div>
        </div>
        <StatusChip
          tone={
            row.phase === 'failed'
              ? 'danger'
              : row.phase === 'done'
                ? 'success'
                : row.phase === 'discarded'
                  ? 'neutral'
                  : 'info'
          }
        >
          {status}
        </StatusChip>
      </div>
      {row.error ? (
        <p className="m-0 text-body-sm text-danger">{t(`uploads.failure.${row.error}` as never)}</p>
      ) : null}
      {row.phase === 'interrupted' && job && (
        <>
          <input
            ref={input}
            type="file"
            className="hidden"
            onChange={(event) => {
              const file = event.currentTarget.files?.[0]
              if (file) setResumeError(!onResume(job, file))
              event.currentTarget.value = ''
            }}
          />
          <Button onClick={() => input.current?.click()}>{t('uploads.resume')}</Button>
          {resumeError && (
            <p className="m-0 text-caption text-danger">{t('uploads.resumeMismatch')}</p>
          )}
        </>
      )}
      {row.phase === 'needs_password' && job && showPassword && (
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            password.mutate(secret, {
              onSuccess: () => {
                setSecret('')
              },
            })
          }}
        >
          <PasswordField
            label={t('uploads.password')}
            value={secret}
            onChange={(event) => {
              setSecret(event.currentTarget.value)
            }}
          />
          {multipartGroup(job.fileName) && (
            <p className="m-0 basis-full text-caption text-ink-2">
              {t('uploads.multipartPasswordHint')}
            </p>
          )}
          <Button type="submit" variant="primary" loading={password.isPending} disabled={!secret}>
            {t('uploads.continue')}
          </Button>
        </form>
      )}
      {(row.phase === 'queued' ||
        row.phase === 'uploading' ||
        row.phase === 'uploadError' ||
        row.phase === 'uploaded' ||
        row.phase === 'extracting' ||
        row.phase === 'filtering') && (
        <ProgressBar
          value={value}
          label={t('uploads.progress', { percent: Math.round(value) })}
          tone={row.phase === 'extracting' || row.phase === 'filtering' ? 'extract' : 'upload'}
        />
      )}
      <div className="flex flex-wrap items-center gap-2">
        {row.phase === 'uploadError' && row.localKey && (
          <Button
            size="sm"
            onClick={() => {
              queue.retry(row.localKey ?? '')
            }}
          >
            {t('common.retry')}
          </Button>
        )}
        {row.phase === 'failed' && job && canRetryJob(job.error) && (
          <Button
            size="sm"
            onClick={() => {
              retry.mutate()
            }}
            loading={retry.isPending}
          >
            {t('common.retry')}
          </Button>
        )}
        {row.phase === 'done' && (
          <a
            className="inline-flex min-h-control-sm items-center rounded-pill border border-accent px-3 text-body-sm font-bold no-underline"
            href="/no-asignados"
            onClick={onOpenPanel}
          >
            {t('uploads.assign')}
          </a>
        )}
        {job && canCancelJob(job.status) && (
          <Button
            size="sm"
            variant="danger"
            onClick={() => void queue.cancel({ key: row.key, jobId: job.id })}
          >
            {t('uploads.cancel')}
          </Button>
        )}
      </div>
      {job && (
        <span className="text-caption text-ink-2">
          {t(`uploads.relation.${job.batch.relation}` as never)}
        </span>
      )}
    </article>
  )
}

function UploadList({ onOpenPanel, demo }: { onOpenPanel: () => void; demo: boolean }) {
  const { t } = useTranslation()
  const jobs = useJobs()
  const queue = useUploadQueue()
  const local = useSyncExternalStore(queue.subscribe, queue.getSnapshot, queue.getSnapshot)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(timer)
    }
  }, [])
  const rows = useMemo(() => buildUploadRows(local, jobs.data ?? [], now), [jobs.data, local, now])
  const jobsById = new Map((jobs.data ?? []).map((job) => [job.id, job]))
  if (jobs.isPending && !rows.length)
    return <p className="text-body-sm text-ink-2">{t('common.loading')}</p>
  if (!rows.length)
    return (
      <EmptyState
        icon={<UploadIcon size={36} />}
        title={t('uploads.emptyTitle')}
        body={t('uploads.emptyBody')}
        action={
          <div className="flex flex-col gap-3">
            {demo && <TryExamplesButton className="inline-flex md:hidden" />}
            <UploadButton />
          </div>
        }
      />
    )
  return (
    <div className="flex flex-col gap-3">
      {rows.map((row) => {
        const job = row.jobId ? jobsById.get(row.jobId) : undefined
        return (
          <UploadRowCard
            key={row.key}
            row={row}
            {...(job ? { job } : {})}
            showPassword={
              !job ||
              isFirstMultipartVolume(
                job.fileName,
                (jobs.data ?? [])
                  .filter((item) => item.batch.id === job.batch.id)
                  .map((item) => item.fileName),
              )
            }
            onResume={(item, file) => queue.resume(item, file)}
            onOpenPanel={onOpenPanel}
          />
        )
      })}
    </div>
  )
}

function UploadPanel({
  open,
  onClose,
  demo,
}: {
  open: boolean
  onClose: () => void
  demo: boolean
}) {
  const { t } = useTranslation()
  const [mobile, setMobile] = useState(() =>
    typeof window === 'undefined' || typeof window.matchMedia !== 'function'
      ? false
      : window.matchMedia('(max-width: 767px)').matches,
  )
  const contents = (
    <div className="flex flex-col gap-3">
      {demo && (
        <>
          <TryExamplesButton className="inline-flex md:hidden" />
          <p className="m-0 rounded-md bg-info-surface p-3 text-body-sm text-info-ink">
            {t('uploads.demoPassword')}
          </p>
        </>
      )}
      <UploadList onOpenPanel={onClose} demo={demo} />
    </div>
  )
  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined
    const media = window.matchMedia('(max-width: 767px)')
    const onChange = () => {
      setMobile(media.matches)
    }
    onChange()
    media.addEventListener('change', onChange)
    return () => {
      media.removeEventListener('change', onChange)
    }
  }, [])
  if (mobile)
    return (
      <Sheet title={t('uploads.title')} open={open} onClose={onClose}>
        {contents}
      </Sheet>
    )
  return (
    <SidePanel title={t('uploads.title')} open={open} onClose={onClose}>
      {contents}
    </SidePanel>
  )
}

export function UploadUiProvider({
  children,
  demo = false,
}: {
  children: ReactNode
  demo?: boolean
}) {
  const { t } = useTranslation()
  const ports = useIngestionPorts()
  const [queue] = useState(() => new UploadQueue(ports))
  const [panelOpen, setPanelOpen] = useState(false)
  const [pending, setPending] = useState<File[] | null>(null)
  const [dragging, setDragging] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const localUploads = useSyncExternalStore(queue.subscribe, queue.getSnapshot, queue.getSnapshot)
  const busy = localUploads.some((item) => item.state === 'queued' || item.state === 'uploading')

  const start = async (
    files: File[],
    relation: 'parts' | 'episodes' | 'none',
    batchName?: string,
  ) => {
    if (!files.length) return
    const totals = multipartPartsTotal(files)
    const name = fileGroupName(files)
    const batch = await ports.createBatch(
      batchName ? { relation, name: batchName } : name ? { relation, name } : { relation },
    )
    queue.add(
      files.map((file) => {
        const group = multipartGroup(file.name)
        const relativePath = (file as DroppedFile).webkitRelativePath
        const partsTotal = group ? totals.get(group) : undefined
        return {
          file,
          spec: {
            batchId: batch.id,
            relation,
            fileName: file.name,
            ...(relativePath ? { relativePath } : {}),
            ...(partsTotal === undefined ? {} : { partsTotal }),
          },
        }
      }),
    )
    setPanelOpen(true)
  }

  const receive = (files: File[]) => {
    if (files.length > 1) setPending(files)
    else void start(files, 'none')
  }

  useEffect(() => {
    const onDragOver = (event: DragEvent) => {
      event.preventDefault()
      setDragging(true)
    }
    const onDragLeave = (event: DragEvent) => {
      if (event.relatedTarget === null) setDragging(false)
    }
    const onDrop = (event: DragEvent) => {
      event.preventDefault()
      setDragging(false)
      if (event.dataTransfer)
        void filesFromDrop(event.dataTransfer)
          .then(receive)
          .catch(() => undefined)
    }
    window.addEventListener('dragover', onDragOver)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', onDrop)
    return () => {
      window.removeEventListener('dragover', onDragOver)
      window.removeEventListener('dragleave', onDragLeave)
      window.removeEventListener('drop', onDrop)
    }
  })
  useEffect(() => {
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!queue.busy()) return
      event.preventDefault()
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload)
    }
  }, [queue])

  return (
    <UploadQueueContext value={queue}>
      <UploadUiContext
        value={{
          openPicker: () => input.current?.click(),
          openExamples: () => {
            const sizedFile = (name: string, size: number) => {
              const file = new File([], name)
              Object.defineProperty(file, 'size', { value: size })
              return file
            }
            setPending([
              sizedFile('Dark.S02E05.1080p.mkv', 1_500_000_000),
              sizedFile('Dark.S02E06.1080p.mkv', 1_480_000_000),
            ])
            void start(
              [
                sizedFile('Inception.2010.part1.rar', 50_000_000),
                sizedFile('Inception.2010.part2.rar', 45_000_000),
              ],
              'parts',
              'sample:Inception',
            )
            const sample = sizedFile('Sample-sample.mkv', 40_000_000)
            Object.defineProperty(sample, 'webkitRelativePath', {
              value: 'Sample/Sample-sample.mkv',
            })
            void start([sample], 'none')
            setPanelOpen(true)
          },
          openPanel: () => {
            setPanelOpen(true)
          },
          busy,
        }}
      >
        {children}
        <input
          ref={input}
          type="file"
          multiple
          className="hidden"
          onChange={(event) => {
            receive(Array.from(event.currentTarget.files ?? []))
            event.currentTarget.value = ''
          }}
        />
        {dragging && (
          <div
            role="status"
            className="fixed inset-4 z-50 flex items-center justify-center rounded-xl border-2 border-dashed border-accent bg-bg/90 text-heading font-bold"
            onDragOver={(event) => {
              event.preventDefault()
            }}
          >
            {t('uploads.dropHere')}
          </div>
        )}
        {pending && (
          <UploadRelationDialog
            files={pending}
            onClose={() => {
              setPending(null)
            }}
            onChoose={(relation) => {
              const files = pending
              setPending(null)
              void start(files, relation)
            }}
          />
        )}
        <UploadPanel
          open={panelOpen}
          demo={demo}
          onClose={() => {
            setPanelOpen(false)
          }}
        />
      </UploadUiContext>
    </UploadQueueContext>
  )
}

export function UploadButton() {
  const { t } = useTranslation()
  const { openPicker } = useUploadUi()
  return (
    <Button icon={<UploadIcon />} variant="primary" onClick={openPicker}>
      {t('library.upload')}
    </Button>
  )
}

export function TryExamplesButton({ className }: { className: string }) {
  const { t } = useTranslation()
  const { openExamples } = useUploadUi()
  return (
    <button
      type="button"
      className={`min-h-control-sm items-center justify-center rounded-pill border border-control px-3 text-center text-caption font-bold md:text-body-sm ${className}`}
      onClick={openExamples}
    >
      {t('uploads.tryExamples')}
    </button>
  )
}
