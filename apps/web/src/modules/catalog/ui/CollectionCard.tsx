import { useTranslation } from 'react-i18next'
import { AppLink } from '@/shared/ui/AppLink'

import { useLibraryCollection } from '../application/queries'

export function CollectionCard({
  id,
  name,
  present,
  image,
}: {
  id: number
  name: string
  present: number
  image?: string
}) {
  const { t } = useTranslation()
  const query = useLibraryCollection(id)
  const total = query.data?.movies.length ?? present
  const inLibrary = query.data
    ? query.data.movies.filter((movie) => movie.inLibrary).length
    : present
  const background = image
    ? `linear-gradient(120deg, rgb(17 34 29 / .86), rgb(12 12 15 / .4)), url(${image})`
    : 'linear-gradient(120deg, #11221d 0%, #2c5246 60%, #0c0c0f 100%)'
  return (
    <AppLink
      href={`/colecciones/${String(id)}`}
      aria-label={`${name}, ${t('library.collectionCount', { present: inLibrary, total })}`}
      className="block min-w-[280px] flex-[0_0_360px] shrink-0 rounded-lg no-underline transition duration-120 ease-out hover:-translate-y-1 focus-visible:-translate-y-1 sm:min-w-0"
      style={{ backgroundImage: background }}
    >
      <span className="flex aspect-video flex-col justify-end gap-1 rounded-lg p-5">
        <strong className="text-[24px] leading-tight font-display">{name}</strong>
        <span className="text-body-sm text-ink-2">
          {t('library.collectionCount', { present: inLibrary, total })}
        </span>
      </span>
    </AppLink>
  )
}
