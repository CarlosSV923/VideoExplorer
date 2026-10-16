import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useLibraryTitles } from '../application/queries'
import { type Kind, sortTitles, type Sort } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import { ChipGroup, EmptyState, PageHeader, SegmentedControl } from '@/shared/ui'
import { LoadingGrid, PosterFor } from './CatalogShared'
import { kindLabel } from './CatalogLabels'

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
        title={kindLabel(kind, t)}
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
          onChange={setSelectedSort}
        />
        <ChipGroup
          legend={t('library.genre')}
          name="genre"
          options={[
            { value: '', label: t('library.allGenres') },
            ...genres.map((value) => ({ value, label: value })),
          ]}
          value={selectedGenre}
          onChange={setSelectedGenre}
        />
      </div>
      {kind === 'series' && (
        <label className="mb-6 flex min-h-control-sm items-center gap-3 text-body">
          <input
            type="checkbox"
            checked={onlyIncomplete}
            onChange={(event) => {
              setOnlyIncomplete(event.currentTarget.checked)
            }}
            className="size-5 accent-accent"
          />
          {t('library.onlyIncomplete')}
        </label>
      )}
      {filtered.length ? (
        <div className="grid grid-cols-2 gap-x-4 gap-y-8 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6">
          {filtered.map((title) => (
            <PosterFor key={title.id} title={title} />
          ))}
        </div>
      ) : (
        <EmptyState title={t('library.emptyFilterTitle')} body={t('library.emptyFilterBody')} />
      )}
    </LibraryLayout>
  )
}
