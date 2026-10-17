import { useMutation, useQueries } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '../application/ports'
import { useInvalidateCatalog } from '../application/queries'
import type { LibraryTitle } from '../domain/types'
import { Button, ConfirmDialog, Menu, StatusChip } from '@/shared/ui'
import { MoreIcon } from '@/shared/ui/icons'
import { useFormat } from '@/shared/i18n/hooks'
import type { Kind } from '../application/search'

export function VersionCard({
  titleId,
  video,
  kind,
}: {
  titleId: number
  video: LibraryTitle['videos'][number]
  kind: Kind
}) {
  const { t } = useTranslation()
  const ports = useCatalogPorts()
  const invalidateCatalog = useInvalidateCatalog()
  const [technical, setTechnical] = useState(false)
  const [confirm, setConfirm] = useState<'move' | 'trash' | null>(null)
  const format = useFormat()
  const move = useMutation({
    mutationFn: () => ports.moveTitleToUnassigned(titleId, { kind: 'file', fileId: video.id }),
    onSuccess: () => {
      setConfirm(null)
      invalidateCatalog()
    },
  })
  const trash = useMutation({
    mutationFn: () => ports.trashTitleScope(titleId, { kind: 'file', fileId: video.id }),
    onSuccess: () => {
      setConfirm(null)
      invalidateCatalog()
    },
  })
  const technicalQuery = useQueries({
    queries: [
      {
        queryKey: ['library', 'technical', titleId, video.id],
        queryFn: () => ports.fileTechnical(titleId, video.id),
        enabled: technical,
      },
    ],
  })[0]
  const fileName = video.path.split('/').at(-1) ?? video.path
  return (
    <article className="flex w-full min-w-0 flex-col gap-3 rounded-lg border border-line bg-surface p-4">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 max-w-full flex-1">
          <h3 className="m-0 break-words text-subheading font-display">
            {video.path.split('/').at(-1)}
          </h3>
          <p className="m-0 mt-1 break-all [overflow-wrap:anywhere] font-mono-file text-mono text-ink-2">
            {video.path}
          </p>
          <p className="m-0 mt-1 text-body-sm text-ink-2">
            {video.part ? t('library.part', { count: video.part }) : ''}
            {video.version ? ` · ${video.version}` : ''} · {format.size(video.size)}
          </p>
        </div>
        <Menu
          label={t('library.fileActionsFor', { name: fileName })}
          items={[
            {
              label: t('library.download'),
              onSelect: () => {
                window.location.assign(ports.downloadFileUrl(titleId, video.id))
              },
            },
            {
              label: t('library.moveUnassigned'),
              onSelect: () => {
                setConfirm('move')
              },
            },
            {
              label: t('library.trash'),
              tone: 'danger',
              onSelect: () => {
                setConfirm('trash')
              },
            },
          ]}
        >
          <MoreIcon />
        </Menu>
      </div>
      {video.subtitles.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {video.subtitles.map((subtitle) => (
            <StatusChip key={subtitle.id} tone="success">
              {subtitle.language}
              {subtitle.forced ? ` · ${t('library.forced')}` : ''}
              {subtitle.sdh ? ` · ${t('library.sdh')}` : ''}
            </StatusChip>
          ))}
        </div>
      )}
      <Button
        variant="secondary"
        size="sm"
        onClick={() => {
          setTechnical((value) => !value)
        }}
      >
        {t('library.technicalInfo')}
      </Button>
      {technical && technicalQuery.data && (
        <dl className="grid auto-fit grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 border-t border-line pt-3 text-body-sm">
          <div>
            <dt className="text-ink-3">{t('library.duration')}</dt>
            <dd className="m-0">{format.duration(technicalQuery.data.duration)}</dd>
          </div>
          <div>
            <dt className="text-ink-3">{t('library.resolution')}</dt>
            <dd className="m-0">
              {technicalQuery.data.video.width} × {technicalQuery.data.video.height}
            </dd>
          </div>
          <div>
            <dt className="text-ink-3">{t('library.codec')}</dt>
            <dd className="m-0">{technicalQuery.data.video.codec}</dd>
          </div>
          <div>
            <dt className="text-ink-3">{t('library.download')}</dt>
            <dd className="m-0">
              <a href={ports.downloadFileUrl(titleId, video.id)}>{t('library.download')}</a>
            </dd>
          </div>
        </dl>
      )}
      {kind === 'movie' && (
        <a href={ports.downloadFileUrl(titleId, video.id)} className="text-body-sm">
          {t('library.download')}
        </a>
      )}
      {confirm === 'move' && (
        <ConfirmDialog
          title={t('library.confirmFileMoveTitle')}
          confirmLabel={t('library.moveUnassigned')}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            move.mutate()
          }}
          busy={move.isPending}
        >
          {t('library.confirmFileMoveBody')}
        </ConfirmDialog>
      )}
      {confirm === 'trash' && (
        <ConfirmDialog
          tone="danger"
          title={t('library.confirmFileTrashTitle')}
          confirmLabel={t('library.trash')}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            trash.mutate()
          }}
          busy={trash.isPending}
        >
          {t('library.confirmFileTrashBody')}
        </ConfirmDialog>
      )}
    </article>
  )
}
