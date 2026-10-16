import { useTranslation } from 'react-i18next'

import { useLibraryTitles, useUnassigned } from '../application/queries'
import { sortTitles } from '../application/search'
import type { LibraryTitleSummary } from '../domain/types'
import { LibraryLayout } from '@/app/LibraryLayout'
import { Button, EmptyState, PosterRow } from '@/shared/ui'
import { UploadIcon } from '@/shared/ui/icons'
import { HeroCarousel } from './HeroCarousel'
import { PendingStrip } from './PendingStrip'
import { LoadingGrid, PosterFor } from './CatalogShared'

export function HomeScreen() {
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
  const recent = sortTitles(titles, 'added')
  const hero = recent.filter((title) => title.backdrop).slice(0, 5)
  const movies = titles.filter((title) => title.kind === 'movie')
  const series = titles.filter((title) => title.kind === 'series')
  const genres = [...new Set(titles.flatMap((title) => title.genres ?? []))]
  const collections = [
    ...new Map(
      titles
        .filter((title) => title.collection)
        .map((title) => [title.collection?.id, title.collection]),
    ).values(),
  ].filter(Boolean)
  const row = (items: LibraryTitleSummary[]) =>
    items.map((title) => <PosterFor key={title.id} title={title} />)
  return (
    <LibraryLayout>
      <h1 className="sr-only">{t('library.home')}</h1>
      {hero.length > 0 && <HeroCarousel titles={hero} />}
      <div className="flex flex-col gap-[var(--row-gap,36px)] py-8">
        <PendingStrip
          titles={titles}
          {...(unassigned.data
            ? { unassignedCount: unassigned.data.entries.length + unassigned.data.jobs.length }
            : {})}
        />
        <PosterRow
          title={t('library.recent')}
          seeAll={
            <a href="/peliculas" className="text-body-sm">
              {t('library.seeAll')}
            </a>
          }
        >
          {row(recent)}
        </PosterRow>
        <PosterRow
          title={t('library.movies')}
          seeAll={
            <a href="/peliculas" className="text-body-sm">
              {t('library.seeAll')}
            </a>
          }
        >
          {row(movies)}
        </PosterRow>
        <PosterRow
          title={t('library.seriesPlural')}
          seeAll={
            <a href="/series" className="text-body-sm">
              {t('library.seeAll')}
            </a>
          }
        >
          {row(series)}
        </PosterRow>
        {genres.map((genre) => (
          <PosterRow key={genre} title={genre}>
            {row(titles.filter((title) => title.genres?.includes(genre)))}
          </PosterRow>
        ))}
        {collections.length > 0 && (
          <PosterRow title={t('library.collections')}>
            {collections.map((collection) => {
              const member = titles.find((title) => title.collection?.id === collection?.id)
              return member ? (
                <PosterFor
                  key={collection?.id}
                  title={member}
                  href={`/colecciones/${String(collection?.id)}`}
                />
              ) : null
            })}
          </PosterRow>
        )}
      </div>
    </LibraryLayout>
  )
}
