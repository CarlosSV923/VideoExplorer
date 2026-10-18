import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '../application/ports'
import { useUnassigned } from '../application/queries'
import type { UploadJob } from '@/modules/ingestion/domain/types'
import { useJobMutation } from '@/modules/ingestion/application/queries'
import { isFirstMultipartVolume, multipartGroup } from '@/modules/ingestion/application/uploadLogic'
import { LibraryLayout } from '@/app/LibraryLayout'
import { isAppError } from '@/shared/kernel/errors'
import { useFormat } from '@/shared/i18n/hooks'
import { Banner, Button, EmptyState, PageHeader, PasswordField, ProgressBar } from '@/shared/ui'
import { UnassignedEntryCard } from './UnassignedEntry'

function JobCard({
  job,
  jobs,
  onRefresh,
}: {
  job: UploadJob
  jobs: readonly UploadJob[]
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const password = useJobMutation('submitPassword', job.id)
  const dismiss = useJobMutation('dismissJob', job.id)
  const [value, setValue] = useState(job.id === 'demo-samba-job-x7k2' ? 'demo' : '')
  const failed = job.status === 'failed'
  const progress =
    job.status === 'extracting' ? job.progress : (job.received / Math.max(1, job.size)) * 100
  const failureKey =
    job.error === 'wrong_password'
      ? 'unassigned.job.failure.wrong_password'
      : job.error === 'archive_corrupt'
        ? 'unassigned.job.failure.archive_corrupt'
        : job.error === 'parts_missing'
          ? 'unassigned.job.failure.parts_missing'
          : 'unassigned.job.failure.unknown'
  return (
    <article className="rounded-xl border border-line bg-surface p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="m-0 break-all text-subheading font-display">{job.fileName}</h2>
          <p className="m-0 mt-1 text-body-sm text-ink-2">
            {t(`unassigned.job.status.${job.status}`)}
          </p>
        </div>
        {failed && (
          <Button
            variant="danger"
            onClick={() => {
              dismiss.mutate(undefined, { onSuccess: onRefresh })
            }}
          >
            {t('unassigned.remove')}
          </Button>
        )}
      </div>
      {job.status === 'needs_password' &&
        isFirstMultipartVolume(
          job.fileName,
          jobs.filter((item) => item.batch.id === job.batch.id).map((item) => item.fileName),
        ) && (
          <form
            className="mt-4 flex flex-wrap items-end gap-3"
            onSubmit={(event) => {
              event.preventDefault()
              password.mutate(value, { onSuccess: onRefresh })
            }}
          >
            <div className="min-w-60 flex-1">
              <PasswordField
                label={t('unassigned.passwordFor', { name: job.fileName })}
                value={value}
                onChange={(event) => {
                  setValue(event.currentTarget.value)
                }}
              />
              {multipartGroup(job.fileName) && (
                <p className="m-0 mt-2 text-caption text-ink-2">
                  {t('uploads.multipartPasswordHint')}
                </p>
              )}
            </div>
            <Button variant="primary" type="submit" loading={password.isPending}>
              {t('unassigned.unlock')}
            </Button>
          </form>
        )}
      {['uploading', 'extracting', 'filtering', 'waiting_parts'].includes(job.status) && (
        <div className="mt-4 flex flex-col gap-2">
          <ProgressBar
            value={progress}
            tone={job.status === 'extracting' ? 'extract' : 'upload'}
            label={t('unassigned.job.progress', { progress: Math.round(progress) })}
          />
          <span className="text-caption text-ink-2">
            {t('unassigned.job.progress', { progress: Math.round(progress) })}
          </span>
        </div>
      )}
      {failed && <p className="mt-4 mb-0 text-body-sm text-danger">{t(failureKey)}</p>}
    </article>
  )
}

export function UnassignedScreen() {
  const { t } = useTranslation()
  const format = useFormat()
  const ports = useCatalogPorts()
  const query = useUnassigned()
  const [error, setError] = useState<unknown>(null)
  const refresh = () => {
    void query.refetch().catch(() => undefined)
  }
  const action = async (fn: () => Promise<void>) => {
    setError(null)
    try {
      await fn()
      refresh()
    } catch (cause) {
      setError(cause)
    }
  }
  if (query.isPending)
    return (
      <LibraryLayout>
        <div className="py-12 text-body text-ink-2">{t('common.loading')}</div>
      </LibraryLayout>
    )
  if (query.isError)
    return (
      <LibraryLayout>
        <div className="py-12">
          <EmptyState
            title={t('errors.loadTitle')}
            body={t('errors.loadBody')}
            action={<Button onClick={refresh}>{t('common.retry')}</Button>}
          />
        </div>
      </LibraryLayout>
    )
  const section = query.data
  const visibleCount = section.entries.length + section.jobs.length
  return (
    <LibraryLayout>
      <PageHeader
        title={t('unassigned.heading')}
        subtitle={t('unassigned.subtitle', { count: visibleCount })}
        actions={<Button onClick={refresh}>{t('unassigned.refresh')}</Button>}
      />
      <div className="flex flex-col gap-5 py-8">
        {error ? (
          <Banner tone="danger">
            {isAppError(error, 'conflict') && error.code === 'assigning'
              ? t('unassigned.errors.assigning')
              : t('errors.loadBody')}
          </Banner>
        ) : null}
        {section.jobs.map((job) => (
          <JobCard key={job.id} job={job} jobs={section.jobs} onRefresh={refresh} />
        ))}
        {section.entries.length === 0 && section.jobs.length === 0 ? (
          <EmptyState title={t('unassigned.empty.title')} body={t('unassigned.empty.body')} />
        ) : (
          section.entries.map((entry) => (
            <UnassignedEntryCard
              key={entry.id}
              entry={entry}
              formatSize={format.size}
              downloadEntryUrl={() => ports.downloadUnassignedEntryUrl(entry.id)}
              downloadFileUrl={(path) => ports.downloadUnassignedFileUrl(entry.id, path)}
              onTrash={() => void action(() => ports.trashUnassignedEntry(entry.id))}
              onDelete={() => void action(() => ports.deleteUnassignedEntry(entry.id))}
              onTrashFile={(path) =>
                void action(() => ports.trashUnassignedFile(entry.id, { path }))
              }
              onDeleteFile={(path) =>
                void action(() => ports.deleteUnassignedFile(entry.id, { path }))
              }
            />
          ))
        )}
      </div>
    </LibraryLayout>
  )
}
