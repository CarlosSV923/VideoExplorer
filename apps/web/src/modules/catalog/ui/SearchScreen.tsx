import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useLibraryTitles } from '../application/queries'
import { matchesSearch } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import { EmptyState, PageHeader, SearchField } from '@/shared/ui'
import { LoadingGrid, PosterFor } from './CatalogShared'

export function SearchScreen({ queryText }: { queryText: string }) {
  const { t } = useTranslation()
  const query = useLibraryTitles()
  const [value, setValue] = useState(queryText)
  const results = query.data?.titles.filter((title) => matchesSearch(title, value)) ?? []
  return (
    <LibraryLayout>
      <PageHeader title={t('library.search')} />
      <div className="py-6">
        <SearchField
          label={t('library.search')}
          value={value}
          onChange={(event) => {
            setValue(event.currentTarget.value)
          }}
          onClear={() => {
            setValue('')
          }}
          placeholder={t('library.searchPlaceholder')}
        />
      </div>
      {query.isPending ? (
        <LoadingGrid />
      ) : results.length ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-x-4 gap-y-8 sm:grid-cols-[repeat(auto-fill,minmax(176px,1fr))]">
          {results.map((title) => (
            <PosterFor key={title.id} title={title} fluid />
          ))}
        </div>
      ) : (
        <EmptyState title={t('library.noResults')} body={t('library.noResultsBody')} />
      )}
    </LibraryLayout>
  )
}
