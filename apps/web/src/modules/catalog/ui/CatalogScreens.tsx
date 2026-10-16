import { EmptyState } from '@/shared/ui'
import { useTranslation } from 'react-i18next'
import { LibraryLayout } from '@/app/LibraryLayout'
import { BrowseScreen } from './BrowseScreen'
import { CollectionScreen } from './CollectionScreen'
import { EditScreen } from './EditScreen'
import { HomeScreen } from './HomeScreen'
import { SearchScreen } from './SearchScreen'
import { TitleDetailScreen } from './TitleDetailScreen'
import { UnassignedScreen } from './UnassignedScreen'
import { AssignScreen } from './AssignScreen'

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
  const { t } = useTranslation()
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
  if (screen === 'unassigned') return <UnassignedScreen />
  if (screen === 'assign' && id) return <AssignScreen id={id} />
  return (
    <LibraryLayout>
      <EmptyState title={t('errors.routeInvalidTitle')} body={t('errors.routeInvalidBody')} />
    </LibraryLayout>
  )
}
