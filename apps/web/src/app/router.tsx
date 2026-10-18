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
import { scheduleCatalogInvalidation, unassignedKey } from '@/modules/catalog/application/queries'
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
  path: '/entrar',
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
        throw redirect({ to: '/entrar', search: { redirect: location.href } })
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
  path: '/peliculas',
  validateSearch: (search: Record<string, unknown>) => ({
    orden: typeof search.orden === 'string' ? search.orden : undefined,
    genero: typeof search.genero === 'string' ? search.genero : undefined,
  }),
  component: MoviesRoute,
})
const seriesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/series',
  validateSearch: (search: Record<string, unknown>) => ({
    orden: typeof search.orden === 'string' ? search.orden : undefined,
    genero: typeof search.genero === 'string' ? search.genero : undefined,
    incompletas: search.incompletas === '1' ? '1' : undefined,
  }),
  component: SeriesRoute,
})
const movieDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/peliculas/$id',
  component: MovieDetailRoute,
})
const seriesDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/series/$id',
  component: SeriesDetailRoute,
})
const movieEditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/peliculas/$id/editar',
  component: MovieEditRoute,
})
const seriesEditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/series/$id/editar',
  component: SeriesEditRoute,
})
const collectionRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/colecciones/$id',
  component: CollectionRoute,
})
const searchRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/buscar',
  validateSearch: (search: Record<string, unknown>) => ({
    q: typeof search.q === 'string' ? search.q : '',
  }),
  component: SearchRoute,
})
const unassignedRoute = screenRoute('/no-asignados', 'unassigned')
const assignRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/no-asignados/$id/asignar',
  component: AssignRoute,
})
const generalSettingsRoute = screenRoute('/ajustes/general', 'generalSettings')
const trashSettingsRoute = screenRoute('/ajustes/papelera', 'trashSettings')

function MoviesRoute() {
  const search = moviesRoute.useSearch()
  return (
    <MarkerScreen
      screen="movies"
      search={{
        ...(search.orden !== undefined ? { orden: search.orden } : {}),
        ...(search.genero !== undefined ? { genero: search.genero } : {}),
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
        ...(search.orden !== undefined ? { orden: search.orden } : {}),
        ...(search.genero !== undefined ? { genero: search.genero } : {}),
        ...(search.incompletas !== undefined ? { incompletas: search.incompletas } : {}),
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
  return <MarkerScreen screen="seriesDetail" id={Number(id)} />
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
