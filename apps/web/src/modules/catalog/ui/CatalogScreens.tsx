import { EmptyState } from '@/shared/ui'
import { LibraryLayout } from '@/app/LibraryLayout'
import { BrowseScreen } from './BrowseScreen'
import { CollectionScreen } from './CollectionScreen'
import { EditScreen } from './EditScreen'
import { HomeScreen } from './HomeScreen'
import { SearchScreen } from './SearchScreen'
import { TitleDetailScreen } from './TitleDetailScreen'

export type CatalogScreenProps = {
  screen: string
  id?: number | undefined
  search?:
    | {
        orden?: string | undefined
        genero?: string | undefined
        incompletas?: string | undefined
        q?: string | undefined
      }
    | undefined
}

export function CatalogScreen({ screen, id, search }: CatalogScreenProps) {
  if (screen === 'home') return <HomeScreen />
  if (screen === 'movies')
    return <BrowseScreen kind="movie" sort={search?.orden} genre={search?.genero} />
  if (screen === 'series')
    return (
      <BrowseScreen
        kind="series"
        sort={search?.orden}
        genre={search?.genero}
        incomplete={search?.incompletas === '1'}
      />
    )
  if (screen === 'movieDetail' && id) return <TitleDetailScreen id={id} kind="movie" />
  if (screen === 'seriesDetail' && id) return <TitleDetailScreen id={id} kind="series" />
  if (screen === 'collection' && id) return <CollectionScreen id={id} />
  if (screen === 'search') return <SearchScreen queryText={search?.q ?? ''} />
  if (screen === 'edit' && id) return <EditScreen id={id} />
  return (
    <LibraryLayout>
      <EmptyState title="" body="" />
    </LibraryLayout>
  )
}
