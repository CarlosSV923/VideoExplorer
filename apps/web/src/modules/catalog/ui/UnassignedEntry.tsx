import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { UnassignedEntry, UnassignedFile } from '../domain/types'
import { ConfirmDialog, LinkButton, Menu, StatusChip } from '@/shared/ui'
import { DownloadIcon } from '@/shared/ui/icons'

export function UnassignedEntryCard({
  entry,
  formatSize,
  downloadFileUrl,
  onTrash,
  onDelete,
  onTrashFile,
  onDeleteFile,
}: {
  entry: UnassignedEntry
  formatSize: (size: number) => string
  downloadFileUrl: (path: string) => string
  onTrash: () => void
  onDelete: () => void
  onTrashFile: (path: string) => void
  onDeleteFile: (path: string) => void
}) {
  const { t } = useTranslation()
  const [confirm, setConfirm] = useState<'entry' | 'file' | null>(null)
  const [selectedFile, setSelectedFile] = useState<UnassignedFile | null>(null)
  const blocked = entry.status !== 'ready'
  const hiddenOnly = entry.files.length === 0
  const status =
    entry.status === 'copying'
      ? { label: t('unassigned.status.copying'), tone: 'warning' as const }
      : entry.status === 'assigning'
        ? { label: t('unassigned.status.assigning'), tone: 'info' as const }
        : { label: t('unassigned.status.ready'), tone: 'success' as const }

  const openFileConfirmation = (file: UnassignedFile) => {
    setSelectedFile(file)
    setConfirm('file')
  }

  const confirmAction = () => {
    if (confirm === 'entry') onDelete()
    if (confirm === 'file' && selectedFile) onDeleteFile(selectedFile.path)
    setConfirm(null)
    setSelectedFile(null)
  }

  return (
    <article className="flex flex-col gap-5 rounded-xl border border-line bg-surface p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h2 className="m-0 break-words text-subheading font-display">{entry.name}</h2>
          <p className="m-0 mt-1 text-body-sm text-ink-2">
            {t(`unassigned.origin.${entry.origin}`)} · {t(`unassigned.relation.${entry.relation}`)}{' '}
            · {t('unassigned.fileCount', { count: entry.files.length })} · {formatSize(entry.size)}
          </p>
        </div>
        <StatusChip tone={status.tone}>{status.label}</StatusChip>
      </div>

      {hiddenOnly ? (
        <div className="rounded-md border border-control bg-surface-2 p-4 text-body-sm text-ink-2">
          <strong className="block text-ink-1">{t('unassigned.hidden.title')}</strong>
          {t('unassigned.hidden.body')}
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {entry.files.map((file) => (
            <div
              key={file.path}
              className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-line bg-surface-2 px-3 py-2"
            >
              <div className="min-w-0">
                <p className="m-0 break-all font-mono-file text-mono text-ink-1">
                  {file.path || entry.name}
                </p>
                <p className="m-0 text-caption text-ink-2">{formatSize(file.size)}</p>
              </div>
              {!blocked && (
                <Menu
                  label={t('unassigned.fileActions', { name: file.path || entry.name })}
                  items={[
                    {
                      label: t('common.download'),
                      onSelect: () => {
                        window.location.assign(downloadFileUrl(file.path))
                      },
                    },
                    {
                      label: t('unassigned.trash'),
                      onSelect: () => {
                        onTrashFile(file.path)
                      },
                    },
                    {
                      label: t('unassigned.delete'),
                      tone: 'danger',
                      onSelect: () => {
                        openFileConfirmation(file)
                      },
                    },
                  ]}
                >
                  {t('common.moreActions')}
                </Menu>
              )}
            </div>
          ))}
        </div>
      )}

      {entry.status === 'copying' && (
        <p className="m-0 text-body-sm text-warning-ink">{t('unassigned.status.copyingBody')}</p>
      )}
      {entry.status === 'assigning' && (
        <p className="m-0 text-body-sm text-info-ink">{t('unassigned.status.assigningBody')}</p>
      )}

      {!blocked && !hiddenOnly && (
        <div className="flex flex-wrap items-center gap-3">
          <LinkButton href={`/no-asignados/${String(entry.id)}/asignar`} variant="primary">
            {t('unassigned.assign')}
          </LinkButton>
          <a
            href={`/api/unassigned/${String(entry.id)}/download`}
            className="inline-flex min-h-control-md items-center gap-2 rounded-pill border border-control px-4 text-body-sm font-bold no-underline"
          >
            <DownloadIcon size={17} />
            {t('common.download')}
          </a>
          <Menu
            label={t('unassigned.entryActions', { name: entry.name })}
            items={[
              { label: t('unassigned.trash'), onSelect: onTrash },
              {
                label: t('unassigned.delete'),
                tone: 'danger',
                onSelect: () => {
                  setConfirm('entry')
                },
              },
            ]}
          >
            {t('common.moreActions')}
          </Menu>
        </div>
      )}

      {confirm && (
        <ConfirmDialog
          tone="danger"
          title={t('unassigned.confirmDelete.title')}
          confirmLabel={t('unassigned.delete')}
          onCancel={() => {
            setConfirm(null)
            setSelectedFile(null)
          }}
          onConfirm={confirmAction}
        >
          {confirm === 'file'
            ? t('unassigned.confirmDelete.fileBody')
            : t('unassigned.confirmDelete.entryBody')}
        </ConfirmDialog>
      )}
    </article>
  )
}
