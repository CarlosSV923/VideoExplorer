import { useTranslation } from 'react-i18next'

import type { LibraryTitle } from '../domain/types'
import { Button, Chip, LinkButton } from '@/shared/ui'
import { EditIcon, RefreshIcon } from '@/shared/ui/icons'
import { kindLabel } from './CatalogLabels'
import { useFormat } from '@/shared/i18n/hooks'

export function TitleHeader({
  title,
  onRefresh,
  refreshing,
}: {
  title: LibraryTitle
  onRefresh: () => void
  refreshing: boolean
}) {
  const { t } = useTranslation()
  const format = useFormat()
  return (
    <section className="relative -mx-[var(--page-x,16px)] overflow-hidden border-b border-line">
      <div className="relative min-h-[360px] p-6 pt-12 md:p-12">
        {title.backdrop && (
          <img src={title.backdrop} alt="" className="absolute inset-0 size-full object-cover" />
        )}
        <div className="absolute inset-0 bg-scrim" />
        <div className="relative flex max-w-4xl flex-col gap-4 md:flex-row md:items-end">
          <div className="hidden w-40 shrink-0 md:block">
            {title.poster ? (
              <img
                src={title.poster}
                alt=""
                className="aspect-[2/3] w-full rounded-md object-cover"
              />
            ) : (
              <div className="aspect-[2/3] rounded-md bg-surface-missing" />
            )}
          </div>
          <div className="flex flex-col gap-3">
            <span className="text-label font-bold uppercase tracking-label text-accent">
              {kindLabel(title.kind, t)}
            </span>
            <h1 className="m-0 text-display font-display">{title.title}</h1>
            <p className="m-0 text-body-sm text-ink-2">
              {title.year || t('library.unknownYear')}
              {title.kind === 'movie' && title.runtime > 0
                ? ` · ${format.duration(title.runtime * 60)}`
                : ''}
              {title.originalTitle ? ` · ${title.originalTitle}` : ''}
            </p>
            {title.genres.length > 0 && (
              <div className="flex flex-wrap gap-2">
                {title.genres.map((genre) => (
                  <Chip key={genre}>{genre}</Chip>
                ))}
              </div>
            )}
            <p className="m-0 max-w-3xl text-body-lg">
              {title.overview || t('library.noSynopsis')}
            </p>
            <div className="flex flex-wrap gap-3">
              <LinkButton
                href={`/peliculas/${String(title.id)}/editar`}
                variant="primary"
                icon={<EditIcon />}
              >
                {title.tmdbId ? t('library.edit') : t('library.link')}
              </LinkButton>
              <Button
                variant="secondary"
                loading={refreshing}
                icon={<RefreshIcon />}
                onClick={onRefresh}
              >
                {t('library.refreshData')}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
