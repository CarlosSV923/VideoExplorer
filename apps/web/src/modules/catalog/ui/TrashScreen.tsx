import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  useDeleteTrashEntry,
  useEmptyTrash,
  usePlanTrashRestore,
  useRestoreTrashEntry,
  useTrash,
} from '../application/queries'
import type { TrashRestoreDecision, TrashRestorePlan } from '../domain/types'
import { LibraryLayout } from '@/app/LibraryLayout'
import { useFormat } from '@/shared/i18n/hooks'
import {
  Button,
  ConfirmDialog,
  Dialog,
  EmptyState,
  PageHeader,
  RestoreIcon,
  StatusChip,
  TrashIcon,
} from '@/shared/ui'

function RestoreDialog({
  plan,
  onClose,
  onRestore,
}: {
  plan: TrashRestorePlan
  onClose: () => void
  onRestore: (
    decisions: { file: string; decision: TrashRestoreDecision; version?: string }[],
  ) => void
}) {
  const { t } = useTranslation()
  const [decisions, setDecisions] = useState(() => new Map<string, TrashRestoreDecision>())
  const [versions, setVersions] = useState(
    () => new Map(plan.files.map((file) => [file.file, file.versionProposal ?? ''])),
  )
  const complete = plan.files.every(
    (file) =>
      file.allowed.length === 0 ||
      (decisions.has(file.file) &&
        (decisions.get(file.file) !== 'keep_both' ||
          !file.versionIncomplete ||
          Boolean(versions.get(file.file)?.trim()))),
  )
  return (
    <Dialog
      title={t('trash.restoreTitle')}
      onClose={onClose}
      size="lg"
      actions={
        <>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
          <Button
            variant="primary"
            disabled={!complete}
            {...(!complete ? { disabledReason: t('trash.restoreDisabled') } : {})}
            onClick={() => {
              onRestore(
                plan.files.flatMap((file) => {
                  const decision = decisions.get(file.file)
                  if (!decision) return []
                  const version = versions.get(file.file)?.trim()
                  return decision === 'keep_both' && version
                    ? [{ file: file.file, decision, version }]
                    : [{ file: file.file, decision }]
                }),
              )
            }}
          >
            {t('trash.restore')}
          </Button>
        </>
      }
    >
      <p className="m-0 text-body-sm text-ink-2">{t('trash.restoreBody')}</p>
      <div className="flex flex-col gap-4">
        {plan.files.map((file) => (
          <article
            key={file.file}
            className="flex flex-col gap-3 rounded-lg border border-line p-4"
          >
            <div className="flex flex-col gap-1">
              <strong className="break-all font-mono text-body-sm">{file.file}</strong>
              <span className="break-all text-caption text-ink-2">
                {file.destinations.join(' · ')}
              </span>
            </div>
            {file.duplicates.length > 0 && (
              <StatusChip tone="warning">{t('trash.conflict')}</StatusChip>
            )}
            {file.allowed.length > 0 && (
              <select
                aria-label={t('trash.decisionFor', { file: file.file })}
                value={decisions.get(file.file) ?? ''}
                onChange={(event) => {
                  const decision = event.currentTarget.value as TrashRestoreDecision
                  setDecisions((current) => new Map(current).set(file.file, decision))
                }}
                className="min-h-control-md rounded-md border border-control bg-surface px-3"
              >
                <option value="">{t('trash.chooseDecision')}</option>
                {file.allowed.includes('replace') && (
                  <option value="replace">{t('trash.replace')}</option>
                )}
                {file.allowed.includes('keep_both') && (
                  <option value="keep_both">{t('trash.keepBoth')}</option>
                )}
                {file.allowed.includes('cancel') && (
                  <option value="cancel">{t('trash.cancel')}</option>
                )}
              </select>
            )}
            {decisions.get(file.file) === 'keep_both' && file.versionIncomplete && (
              <input
                aria-label={t('trash.versionFor', { file: file.file })}
                value={versions.get(file.file)}
                onChange={(event) => {
                  setVersions((current) =>
                    new Map(current).set(file.file, event.currentTarget.value),
                  )
                }}
                className="min-h-control-md rounded-md border border-control bg-surface px-3 font-mono"
                placeholder={t('trash.version')}
              />
            )}
          </article>
        ))}
      </div>
    </Dialog>
  )
}

export function TrashScreen() {
  const { t } = useTranslation()
  const format = useFormat()
  const trash = useTrash()
  const empty = useEmptyTrash()
  const remove = useDeleteTrashEntry()
  const planRestore = usePlanTrashRestore()
  const restore = useRestoreTrashEntry()
  const [confirm, setConfirm] = useState<'empty' | number | null>(null)
  const [plan, setPlan] = useState<TrashRestorePlan | null>(null)
  const sorted = useMemo(
    () =>
      [...(trash.data?.entries ?? [])].sort(
        (a, b) => Date.parse(b.trashedAt) - Date.parse(a.trashedAt),
      ),
    [trash.data],
  )
  return (
    <LibraryLayout>
      <PageHeader
        title={t('screens.trashSettings.title')}
        actions={
          sorted.length ? (
            <Button
              variant="danger"
              icon={<TrashIcon />}
              onClick={() => {
                setConfirm('empty')
              }}
            >
              {t('trash.empty')}
            </Button>
          ) : undefined
        }
      />
      <div className="page-padding py-6">
        {!sorted.length ? (
          <EmptyState
            icon={<TrashIcon size={40} />}
            title={t('trash.emptyTitle')}
            body={t('trash.emptyBody')}
          />
        ) : (
          <div className="flex flex-col gap-3">
            {sorted.map((entry) => {
              const entryName = entry.files.map((file) => file.name).join(', ')
              return (
                <article
                  key={entry.id}
                  className="flex flex-col gap-4 rounded-lg border border-line bg-surface-2 p-5"
                >
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="min-w-0">
                      <h2 className="m-0 break-words text-heading">{entryName}</h2>
                      <p className="m-0 mt-2 text-body-sm text-ink-2">
                        {t(`trash.origin.${entry.origin}` as never)} ·{' '}
                        {t(`trash.reason.${entry.reason}` as never)}
                      </p>
                      {entry.relativePath && (
                        <p className="m-0 mt-1 break-all font-mono text-caption text-ink-3">
                          {entry.relativePath}
                        </p>
                      )}
                    </div>
                    <div className="text-right text-body-sm text-ink-2">
                      <div>{format.date(entry.trashedAt)}</div>
                      <div>{t('trash.purgeAt', { date: format.date(entry.purgeAt) })}</div>
                    </div>
                  </div>
                  <ul className="m-0 list-none border-t border-line pt-3 text-body-sm text-ink-2">
                    {entry.files.map((file) => (
                      <li key={file.name} className="flex justify-between gap-3">
                        <span className="break-all font-mono">{file.name}</span>
                        <span className="shrink-0">{format.size(file.size)}</span>
                      </li>
                    ))}
                  </ul>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      icon={<RestoreIcon />}
                      aria-label={t('trash.restoreFor', { name: entryName })}
                      onClick={() => {
                        planRestore.mutate(entry.id, { onSuccess: setPlan })
                      }}
                      loading={planRestore.isPending && planRestore.variables === entry.id}
                    >
                      {t('trash.restore')}
                    </Button>
                    <Button
                      variant="danger"
                      aria-label={t('trash.deleteForeverFor', { name: entryName })}
                      onClick={() => {
                        setConfirm(entry.id)
                      }}
                    >
                      {t('trash.deleteForever')}
                    </Button>
                  </div>
                </article>
              )
            })}
          </div>
        )}
      </div>
      {plan && (
        <RestoreDialog
          plan={plan}
          onClose={() => {
            setPlan(null)
          }}
          onRestore={(decisions) => {
            restore.mutate(
              { id: plan.entryId, request: { decisions } },
              {
                onSuccess: () => {
                  setPlan(null)
                },
              },
            )
          }}
        />
      )}
      {confirm !== null && (
        <ConfirmDialog
          tone="danger"
          title={confirm === 'empty' ? t('trash.emptyConfirmTitle') : t('trash.deleteConfirmTitle')}
          confirmLabel={confirm === 'empty' ? t('trash.empty') : t('trash.deleteForever')}
          busy={empty.isPending || remove.isPending}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            if (confirm === 'empty')
              empty.mutate(undefined, {
                onSettled: () => {
                  setConfirm(null)
                },
              })
            else
              remove.mutate(confirm, {
                onSettled: () => {
                  setConfirm(null)
                },
              })
          }}
        >
          {confirm === 'empty'
            ? t('trash.emptyConfirmBody', { count: sorted.length })
            : t('trash.deleteConfirmBody')}
        </ConfirmDialog>
      )}
    </LibraryLayout>
  )
}
