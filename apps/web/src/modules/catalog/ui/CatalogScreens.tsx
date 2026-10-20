import { lazy, Suspense } from 'react'
import { EmptyState } from '@/shared/ui'
import { useTranslation } from 'react-i18next'
import { LibraryLayout } from '@/app/LibraryLayout'
import { RouteLoading } from '@/app/RouteLoading'

const BrowseScreen = lazy(() =>
  import('./BrowseScreen').then((module) => ({ default: module.BrowseScreen })),
)
const CollectionScreen = lazy(() =>
  import('./CollectionScreen').then((module) => ({ default: module.CollectionScreen })),
)
const EditScreen = lazy(() =>
  import('./EditScreen').then((module) => ({ default: module.EditScreen })),
)
const HomeScreen = lazy(() =>
  import('./HomeScreen').then((module) => ({ default: module.HomeScreen })),
)
const SearchScreen = lazy(() =>
  import('./SearchScreen').then((module) => ({ default: module.SearchScreen })),
)
const TitleDetailScreen = lazy(() =>
  import('./TitleDetailScreen').then((module) => ({ default: module.TitleDetailScreen })),
)
const UnassignedScreen = lazy(() =>
  import('./UnassignedScreen').then((module) => ({ default: module.UnassignedScreen })),
)
const AssignScreen = lazy(() =>
  import('./AssignScreen').then((module) => ({ default: module.AssignScreen })),
)

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
  random?: (() => number) | undefined
}

export function CatalogScreen({ screen, id, search, random }: CatalogScreenProps) {
  const { t } = useTranslation()
  if (screen === 'home')
    return (
      <Suspense fallback={<RouteLoading />}>
        <HomeScreen {...(random ? { random } : {})} />
      </Suspense>
    )
  if (screen === 'movies')
    return (
      <Suspense fallback={<RouteLoading />}>
        <BrowseScreen kind="movie" sort={search?.orden} genre={search?.genero} />
      </Suspense>
    )
  if (screen === 'series')
    return (
      <Suspense fallback={<RouteLoading />}>
        <BrowseScreen
          kind="series"
          sort={search?.orden}
          genre={search?.genero}
          incomplete={search?.incompletas === '1'}
        />
      </Suspense>
    )
  if (screen === 'movieDetail' && id)
    return (
      <Suspense fallback={<RouteLoading />}>
        <TitleDetailScreen id={id} kind="movie" />
      </Suspense>
    )
  if (screen === 'seriesDetail' && id)
    return (
      <Suspense fallback={<RouteLoading />}>
        <TitleDetailScreen id={id} kind="series" />
      </Suspense>
    )
  if (screen === 'collection' && id)
    return (
      <Suspense fallback={<RouteLoading />}>
        <CollectionScreen id={id} />
      </Suspense>
    )
  if (screen === 'search')
    return (
      <Suspense fallback={<RouteLoading />}>
        <SearchScreen queryText={search?.q ?? ''} />
      </Suspense>
    )
  if (screen === 'edit' && id)
    return (
      <Suspense fallback={<RouteLoading />}>
        <EditScreen id={id} />
      </Suspense>
    )
  if (screen === 'unassigned')
    return (
      <Suspense fallback={<RouteLoading />}>
        <UnassignedScreen />
      </Suspense>
    )
  if (screen === 'assign' && id)
    return (
      <Suspense fallback={<RouteLoading />}>
        <AssignScreen id={id} />
      </Suspense>
    )
  return (
    <LibraryLayout>
      <EmptyState title={t('errors.routeInvalidTitle')} body={t('errors.routeInvalidBody')} />
    </LibraryLayout>
  )
}
