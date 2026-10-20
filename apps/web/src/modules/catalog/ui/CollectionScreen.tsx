import { useTranslation } from 'react-i18next'

import { useLibraryCollection } from '../application/queries'
import { LibraryLayout } from '@/app/LibraryLayout'
import { EmptyState, LinkButton, PageHeader, Poster } from '@/shared/ui'
import { isAppError } from '@/shared/kernel/errors'
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
          {isAppError(query.error, 'notFound') ? (
            <EmptyState
              title={t('missing.collectionTitle')}
              body={t('missing.collection')}
              action={
                <LinkButton href="/peliculas" variant="primary">
                  {t('missing.backMovies')}
                </LinkButton>
              }
            />
          ) : (
            <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
          )}
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
      <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-x-4 gap-y-8 py-8 sm:grid-cols-[repeat(auto-fill,minmax(176px,1fr))]">
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
            fluid
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
