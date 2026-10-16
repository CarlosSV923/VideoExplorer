import { useTranslation } from 'react-i18next'

import { useLibraryCollection } from '../application/queries'
import { LibraryLayout } from '@/app/LibraryLayout'
import { EmptyState, PageHeader, Poster } from '@/shared/ui'
import { LoadingGrid } from './CatalogShared'

export function CollectionScreen({ id }: { id: number }) {
  const { t } = useTranslation()
  const query = useLibraryCollection(id)
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
  const collection = query.data
  const present = collection.movies.filter((movie) => movie.inLibrary).length
  return (
    <LibraryLayout>
      <PageHeader
        title={collection.name}
        subtitle={t('library.collectionCount', {
          present,
          total: collection.movies.length,
        })}
      />
      <div className="grid grid-cols-2 gap-x-4 gap-y-8 py-8 sm:grid-cols-3 lg:grid-cols-5">
        {collection.movies.map((movie) => (
          <Poster
            key={movie.id}
            title={movie.title}
            meta={[
              movie.year || undefined,
              t(movie.inLibrary ? 'library.inLibrary' : 'library.notInLibrary'),
            ]
              .filter(Boolean)
              .join(' · ')}
            dimmed={!movie.inLibrary}
            {...(movie.poster ? { image: movie.poster } : {})}
            {...(movie.libraryTitleId
              ? { href: `/peliculas/${String(movie.libraryTitleId)}` }
              : {})}
          />
        ))}
      </div>
    </LibraryLayout>
  )
}
