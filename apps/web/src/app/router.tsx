/* eslint-disable react-refresh/only-export-components -- route definitions */
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  useRouter,
  type RouterHistory,
} from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import type { IdentityPorts } from '@/modules/identity/application/ports'
import { sessionKey, sessionQuery } from '@/modules/identity/application/queries'
import { LoginScreen } from '@/modules/identity/ui/LoginScreen'
import { useIngestionPorts } from '@/modules/ingestion/application/ports'
import { watchJobsInCache } from '@/modules/ingestion/application/queries'
import {
  libraryTitleKey,
  libraryTitlesKey,
  scheduleCatalogInvalidation,
  unassignedKey,
} from '@/modules/catalog/application/queries'
import { isAppError } from '@/shared/kernel/errors'
import { Button, EmptyState } from '@/shared/ui'
import { useEffect } from 'react'
import { SystemHealthBanner } from '@/modules/system/ui/SystemHealthBanner'
import { dataSource } from './config'

import { MarkerScreen, type ScreenKey } from './MarkerScreen'
import type { Services } from './services'

export interface RouterContext {
  queryClient: QueryClient
  identity: IdentityPorts
  services: Services
}

export function safeRedirect(target: unknown): string {
  return typeof target === 'string' && target.startsWith('/') && !target.startsWith('//')
    ? target
    : '/'
}

function RouteError() {
  const { t } = useTranslation()
  const router = useRouter()
  return (
    <main className="flex min-h-app items-center justify-center p-6">
      <EmptyState
        title={t('errors.loadTitle')}
        body={t('errors.loadBody')}
        action={
          <Button variant="primary" onClick={() => void router.invalidate().catch(() => undefined)}>
            {t('common.retry')}
          </Button>
        }
      />
    </main>
  )
}

function NotFound() {
  const { t } = useTranslation()
  return (
    <main className="flex min-h-app items-center justify-center p-6">
      <EmptyState title={t('notFound.title')} body={t('notFound.body')} />
    </main>
  )
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
  notFoundComponent: NotFound,
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (search: Record<string, unknown>): { redirect?: string } =>
    typeof search.redirect === 'string' ? { redirect: search.redirect } : {},
  beforeLoad: ({ context, search }) => {
    if (context.queryClient.getQueryData(sessionKey)) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- the router redirect is a control-flow object
      throw redirect({ href: safeRedirect(search.redirect) })
    }
  },
  component: function Login() {
    const router = useRouter()
    const { redirect: target } = loginRoute.useSearch()
    return (
      <LoginScreen
        onSuccess={() => {
          void router.navigate({ to: safeRedirect(target), replace: true })
        }}
      />
    )
  },
})

function AppShell() {
  const { t } = useTranslation()
  const ports = useIngestionPorts()
  const queryClient = useQueryClient()
  useEffect(() => {
    if (typeof EventSource === 'undefined' || typeof ports.watchJobs !== 'function')
      return undefined
    return watchJobsInCache(queryClient, ports, {
      onCatalogChanged: (titleId) => {
        scheduleCatalogInvalidation(queryClient, [unassignedKey, libraryTitlesKey])
        if (titleId !== undefined)
          scheduleCatalogInvalidation(queryClient, [libraryTitleKey(titleId)])
      },
      onStatusChange: (job) => {
        if (['done', 'discarded', 'failed', 'cancelled'].includes(job.status)) {
          scheduleCatalogInvalidation(queryClient, [unassignedKey])
        }
      },
    })
  }, [ports, queryClient])
  return (
    <>
      {dataSource === 'demo' && (
        <div
          role="status"
          className="sticky top-16 z-20 border-b border-info bg-info-surface px-[var(--page-x,16px)] py-2 text-center text-body-sm text-info-ink"
        >
          {t('demo.banner')}
        </div>
      )}
      <SystemHealthBanner />
      <Outlet />
    </>
  )
}

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: async ({ context, location }) => {
    try {
      await context.queryClient.query(sessionQuery(context.identity))
    } catch (error) {
      if (isAppError(error, 'unauthorized')) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- the router redirect is a control-flow object
        throw redirect({ to: '/login', search: { redirect: location.href } })
      }
      throw error
    }
  },
  component: AppShell,
})

function screenRoute(path: string, screen: ScreenKey) {
  return createRoute({
    getParentRoute: () => appRoute,
    path,
    component: () => <MarkerScreen screen={screen} />,
  })
}

const homeRoute = screenRoute('/', 'home')
const moviesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/movies',
  validateSearch: (search: Record<string, unknown>) => ({
    sort: typeof search.sort === 'string' ? search.sort : undefined,
    genre: typeof search.genre === 'string' ? search.genre : undefined,
  }),
  component: MoviesRoute,
})
const seriesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/series',
  validateSearch: (search: Record<string, unknown>) => ({
    sort: typeof search.sort === 'string' ? search.sort : undefined,
    genre: typeof search.genre === 'string' ? search.genre : undefined,
    incomplete: search.incomplete === 1 ? 1 : undefined,
  }),
  component: SeriesRoute,
})
const movieDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/movies/$id',
  component: MovieDetailRoute,
})
const seriesDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/series/$id',
  validateSearch: (search: Record<string, unknown>) => ({
    season: typeof search.season === 'number' ? search.season : undefined,
  }),
  component: SeriesDetailRoute,
})
const movieEditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/movies/$id/edit',
  component: MovieEditRoute,
})
const seriesEditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/series/$id/edit',
  component: SeriesEditRoute,
})
const collectionRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/collections/$id',
  component: CollectionRoute,
})
const searchRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/search',
  validateSearch: (search: Record<string, unknown>) => ({
    q: typeof search.q === 'string' ? search.q : '',
  }),
  component: SearchRoute,
})
const unassignedRoute = screenRoute('/unassigned', 'unassigned')
const assignRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/unassigned/$id/assign',
  component: AssignRoute,
})
const generalSettingsRoute = screenRoute('/settings/general', 'generalSettings')
const trashSettingsRoute = screenRoute('/settings/trash', 'trashSettings')

function MoviesRoute() {
  const search = moviesRoute.useSearch()
  return (
    <MarkerScreen
      screen="movies"
      search={{
        ...(search.sort !== undefined ? { sort: search.sort } : {}),
        ...(search.genre !== undefined ? { genre: search.genre } : {}),
      }}
    />
  )
}

function SeriesRoute() {
  const search = seriesRoute.useSearch()
  return (
    <MarkerScreen
      screen="series"
      search={{
        ...(search.sort !== undefined ? { sort: search.sort } : {}),
        ...(search.genre !== undefined ? { genre: search.genre } : {}),
        ...(search.incomplete !== undefined ? { incomplete: String(search.incomplete) } : {}),
      }}
    />
  )
}

function MovieDetailRoute() {
  const { id } = movieDetailRoute.useParams()
  return <MarkerScreen screen="movieDetail" id={Number(id)} />
}

function SeriesDetailRoute() {
  const { id } = seriesDetailRoute.useParams()
  const search = seriesDetailRoute.useSearch()
  return <MarkerScreen screen="seriesDetail" id={Number(id)} search={search} />
}

function MovieEditRoute() {
  const { id } = movieEditRoute.useParams()
  return <MarkerScreen screen="edit" id={Number(id)} />
}

function SeriesEditRoute() {
  const { id } = seriesEditRoute.useParams()
  return <MarkerScreen screen="edit" id={Number(id)} />
}

function AssignRoute() {
  const { id } = assignRoute.useParams()
  return <MarkerScreen screen="assign" id={Number(id)} />
}

function CollectionRoute() {
  const { id } = collectionRoute.useParams()
  return <MarkerScreen screen="collection" id={Number(id)} />
}

function SearchRoute() {
  const search = searchRoute.useSearch()
  return <MarkerScreen screen="search" search={search} />
}

const routeTree = rootRoute.addChildren([
  loginRoute,
  appRoute.addChildren([
    homeRoute,
    moviesRoute,
    seriesRoute,
    movieDetailRoute,
    seriesDetailRoute,
    movieEditRoute,
    seriesEditRoute,
    collectionRoute,
    searchRoute,
    unassignedRoute,
    assignRoute,
    generalSettingsRoute,
    trashSettingsRoute,
  ]),
])

export function createAppRouter(context: RouterContext, history?: RouterHistory) {
  return createRouter({
    routeTree,
    context,
    ...(history ? { history } : {}),
    scrollRestoration: true,
    defaultErrorComponent: RouteError,
    defaultPendingMs: 300,
  })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
