import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'

import type { LibraryTitleSummary } from '../domain/types'
import { Poster, StatusChip } from '@/shared/ui'
import { kindLabel } from './CatalogLabels'

export function LoadingGrid() {
  return (
    <div
      aria-hidden="true"
      className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6"
    >
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="aspect-[2/3] animate-pulse rounded-md bg-surface" />
      ))}
    </div>
  )
}

export function PosterFor({ title, href }: { title: LibraryTitleSummary; href?: string }) {
  const { t } = useTranslation()
  const chips = [
    title.missingEpisodes !== undefined ? (
      <StatusChip key="missing" tone="warning" solid>
        {t('library.missingCount', { count: title.missingEpisodes })}
      </StatusChip>
    ) : null,
    !title.tmdbId ? (
      <StatusChip key="free" tone="warning" solid>
        {t('library.unlinked')}
      </StatusChip>
    ) : null,
  ].filter(Boolean) as ReactNode[]
  const meta = [title.year || undefined, kindLabel(title.kind, t)].filter(Boolean).join(' · ')
  return (
    <Poster
      title={title.title}
      meta={meta}
      chips={chips}
      accessibleChips={chips.map(() => '')}
      seed={String(title.id)}
      {...(title.poster ? { image: title.poster } : {})}
      href={href ?? `/${title.kind === 'movie' ? 'peliculas' : 'series'}/${String(title.id)}`}
    />
  )
}
