import { useTranslation } from 'react-i18next'

import type { LibraryTitle } from '../domain/types'
import { Poster } from '@/shared/ui'

export function CollectionInline({
  collection,
}: {
  collection: NonNullable<LibraryTitle['collection']>
}) {
  const { t } = useTranslation()
  const present = collection.movies.filter((movie) => movie.inLibrary).length
  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="m-0 text-heading font-display">{collection.name}</h2>
          <p className="m-0 mt-1 text-body-sm text-ink-2">
            {t('library.collectionCount', { present, total: collection.movies.length })}
          </p>
        </div>
        <a href={`/colecciones/${String(collection.id)}`}>{t('library.viewCollection')}</a>
      </div>
      <div className="mt-3 flex gap-4 overflow-x-auto">
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
    </section>
  )
}
