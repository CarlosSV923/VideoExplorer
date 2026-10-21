import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useLibraryTitles, useUnassigned } from '../application/queries'
import { sortTitles } from '../application/search'
import type { LibraryTitleSummary } from '../domain/types'
import { LibraryLayout } from '@/app/LibraryLayout'
import { AutoPosterRow, Button, EmptyState } from '@/shared/ui'
import { UploadIcon } from '@/shared/ui/icons'
import { HeroCarousel } from './HeroCarousel'
import { PendingStrip } from './PendingStrip'
import { LoadingGrid, PosterFor } from './CatalogShared'
import { selectHeroTitles } from './heroSelection'
import { AppLink } from '@/shared/ui/AppLink'

export function HomeScreen({ random = Math.random }: { random?: () => number }) {
  const { t } = useTranslation()
  const query = useLibraryTitles()
  const unassigned = useUnassigned()
  if (query.isPending)
    return (
      <LibraryLayout>
        <div className="py-12">
          <LoadingGrid />
        </div>
      </LibraryLayout>
    )
  if (query.isError)
    return (
      <LibraryLayout>
        <div className="py-12">
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )
  const titles = query.data.titles
  if (!titles.length)
    return (
      <LibraryLayout>
        <div className="flex min-h-[70vh] flex-col justify-center gap-5">
          <h1 className="sr-only">{t('library.home')}</h1>
          <EmptyState
            title={t('library.emptyTitle')}
            body={t('library.emptyBody')}
            action={
              <Button variant="primary" icon={<UploadIcon />}>
                {t('library.upload')}
              </Button>
            }
          />
        </div>
      </LibraryLayout>
    )
  return (
    <HomeContent
      titles={titles}
      {...(unassigned.data
        ? { pending: unassigned.data.entries.length + unassigned.data.jobs.length }
        : {})}
      random={random}
    />
  )
}

function HomeContent({
  titles,
  pending,
  random,
}: {
  titles: LibraryTitleSummary[]
  pending?: number
  random: () => number
}) {
  const { t } = useTranslation()
  const recent = sortTitles(titles, 'added')
  const movies = recent.filter((title) => title.kind === 'movie').slice(0, 10)
  const series = recent.filter((title) => title.kind === 'series').slice(0, 10)
  const [hero] = useState(() => selectHeroTitles([...movies, ...series], random))
  const row = (items: LibraryTitleSummary[]) =>
    items.map((title) => <PosterFor key={title.id} title={title} />)
  return (
    <LibraryLayout>
      <h1 className="sr-only">{t('library.home')}</h1>
      {hero.length > 0 && <HeroCarousel titles={hero} />}
      <div className="flex flex-col gap-[var(--row-gap,36px)] py-8">
        <PendingStrip
          titles={titles}
          {...(pending === undefined ? {} : { unassignedCount: pending })}
        />
        {movies.length > 0 && (
          <AutoPosterRow
            title={t('library.recentMovies')}
            items={row(movies)}
            seeAll={
              <AppLink href="/peliculas" className="text-body-sm">
                {t('library.seeAll')}
              </AppLink>
            }
          />
        )}
        {series.length > 0 && (
          <AutoPosterRow
            title={t('library.recentSeries')}
            items={row(series)}
            seeAll={
              <AppLink href="/series" className="text-body-sm">
                {t('library.seeAll')}
              </AppLink>
            }
          />
        )}
      </div>
    </LibraryLayout>
  )
}
