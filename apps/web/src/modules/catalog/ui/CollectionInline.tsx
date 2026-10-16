import { useTranslation } from 'react-i18next'

import type { LibraryTitle } from '../domain/types'
import { Poster } from '@/shared/ui'

export function CollectionInline({
  collection,
}: {
  collection: NonNullable<LibraryTitle['collection']>
}) {
  const { t } = useTranslation()
  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-heading font-display">{t('library.collection')}</h2>
        <a href={`/colecciones/${String(collection.id)}`}>{t('library.viewCollection')}</a>
      </div>
      <div className="mt-3 flex gap-4 overflow-x-auto">
        {collection.movies.map((movie) => (
          <Poster
            key={movie.id}
            title={movie.title}
            meta={String(movie.year)}
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
