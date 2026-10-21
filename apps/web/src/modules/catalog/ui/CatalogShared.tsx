import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'

import type { LibraryTitleSummary } from '../domain/types'
import { Poster, StatusChip } from '@/shared/ui'
import { kindLabel } from './CatalogLabels'

export function LoadingGrid() {
  return (
    <div
      aria-hidden="true"
      className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-4 sm:grid-cols-[repeat(auto-fill,minmax(176px,1fr))]"
    >
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="aspect-[2/3] animate-pulse rounded-md bg-surface" />
      ))}
    </div>
  )
}

export function PosterFor({
  title,
  href,
  fluid = false,
}: {
  title: LibraryTitleSummary
  href?: string
  fluid?: boolean
}) {
  const { t } = useTranslation()
  const chips = [
    title.missingEpisodes !== undefined && title.missingEpisodes > 0 ? (
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
      fluid={fluid}
      {...(title.poster ? { image: title.poster } : {})}
      href={href ?? `/${title.kind === 'movie' ? 'movies' : 'series'}/${String(title.id)}`}
    />
  )
}
