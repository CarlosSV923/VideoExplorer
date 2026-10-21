import { useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { useLibraryTitles } from '../application/queries'
import { type Kind, sortTitles, type Sort } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import { ChipGroup, EmptyState, PageHeader, SegmentedControl } from '@/shared/ui'
import { LoadingGrid, PosterFor } from './CatalogShared'

export function BrowseScreen({
  kind,
  sort = 'added',
  genre,
  incomplete = false,
}: {
  kind: Kind
  sort?: string | undefined
  genre?: string | undefined
  incomplete?: boolean | undefined
}) {
  const { t } = useTranslation()
  const router = useRouter({ warn: false }) as ReturnType<typeof useRouter> | undefined
  const query = useLibraryTitles()
  const [selectedSort, setSelectedSort] = useState<Sort>(
    sort === 'title' || sort === 'year' ? sort : 'added',
  )
  const [selectedGenre, setSelectedGenre] = useState(genre ?? '')
  const [onlyIncomplete, setOnlyIncomplete] = useState(incomplete)
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
  const all = query.data.titles.filter((title) => title.kind === kind)
  const genres = [...new Set(all.flatMap((title) => title.genres ?? []))]
  const filtered = sortTitles(
    all.filter(
      (title) =>
        (!selectedGenre || title.genres?.includes(selectedGenre)) &&
        (!onlyIncomplete || (title.missingEpisodes ?? 0) > 0),
    ),
    selectedSort,
  )
  return (
    <LibraryLayout>
      <PageHeader
        title={kind === 'movie' ? t('library.movies') : t('library.seriesPlural')}
        subtitle={t('library.titleCount', { count: filtered.length })}
      />
      <div className="flex flex-wrap items-center justify-between gap-4 py-6">
        <SegmentedControl
          label={t('library.sort')}
          options={[
            { value: 'added', label: t('library.sortAdded') },
            { value: 'title', label: t('library.sortTitle') },
            { value: 'year', label: t('library.sortYear') },
          ]}
          value={selectedSort}
          onChange={(value) => {
            setSelectedSort(value)
            const search = { sort: value, genre: selectedGenre || undefined }
            if (kind === 'movie') void router?.navigate({ to: '/movies', search })
            else
              void router?.navigate({
                to: '/series',
                search: { ...search, incomplete: onlyIncomplete ? 1 : undefined },
              })
          }}
        />
        <ChipGroup
          legend={t('library.genre')}
          name="genre"
          options={[
            { value: '', label: t('library.allGenres') },
            ...genres.map((value) => ({ value, label: value })),
          ]}
          value={selectedGenre}
          onChange={(value) => {
            setSelectedGenre(value)
            const search = { sort: selectedSort, genre: value || undefined }
            if (kind === 'movie') void router?.navigate({ to: '/movies', search })
            else
              void router?.navigate({
                to: '/series',
                search: { ...search, incomplete: onlyIncomplete ? 1 : undefined },
              })
          }}
        />
      </div>
      {kind === 'series' && (
        <label className="mb-6 flex min-h-control-sm items-center gap-3 text-body">
          <input
            type="checkbox"
            checked={onlyIncomplete}
            onChange={(event) => {
              const incomplete = event.currentTarget.checked
              setOnlyIncomplete(incomplete)
              void router?.navigate({
                to: '/series',
                search: {
                  sort: selectedSort,
                  genre: selectedGenre || undefined,
                  incomplete: incomplete ? 1 : undefined,
                },
              })
            }}
            className="size-5 accent-accent"
          />
          {t('library.onlyIncomplete')}
        </label>
      )}
      {filtered.length ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-x-4 gap-y-8 sm:grid-cols-[repeat(auto-fill,minmax(176px,1fr))]">
          {filtered.map((title) => (
            <PosterFor key={title.id} title={title} fluid />
          ))}
        </div>
      ) : (
        <EmptyState title={t('library.emptyFilterTitle')} body={t('library.emptyFilterBody')} />
      )}
    </LibraryLayout>
  )
}
