import type { i18n as I18n } from 'i18next'
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, type RouterHistory } from '@tanstack/react-router'
import { lazy, Suspense, useEffect, useState, type ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'

import { CatalogContext } from '@/modules/catalog/application/ports'
import { IdentityContext } from '@/modules/identity/application/ports'
import { sessionKey } from '@/modules/identity/application/queries'
import { IngestionContext } from '@/modules/ingestion/application/ports'
import { MediaServersContext } from '@/modules/mediaservers/application/ports'
import { MetadataContext } from '@/modules/metadata/application/ports'
import { SystemContext } from '@/modules/system/application/ports'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { isAppError } from '@/shared/kernel/errors'
import { RouteLoading } from './RouteLoading'

const UploadUiProvider = lazy(() =>
  import('@/modules/ingestion/ui/UploadCenter').then((module) => ({
    default: module.UploadUiProvider,
  })),
)

import { dataSource } from './config'
import { createAppRouter } from './router'
import { createServices, type Services } from './services'

function createEnvironment(services: Services, history: RouterHistory | undefined) {
  let onUnauthorized = () => undefined
  const handleError = (error: unknown) => {
    if (isAppError(error, 'unauthorized')) onUnauthorized()
  }
  const queryClient = new QueryClient({
    queryCache: new QueryCache({ onError: handleError }),
    mutationCache: new MutationCache({ onError: handleError }),
    defaultOptions: { queries: { retry: false, staleTime: 10_000 }, mutations: { retry: false } },
  })
  const router = createAppRouter({ queryClient, identity: services.identity, services }, history)
  onUnauthorized = () => {
    const { pathname, href } = router.state.location
    if (pathname === '/entrar') return
    queryClient.removeQueries({ queryKey: sessionKey })
    void router.navigate({ to: '/entrar', search: { redirect: href }, replace: true })
  }
  return { queryClient, router }
}

function Ports({ services, children }: { services: Services; children: ReactNode }) {
  return (
    <IdentityContext value={services.identity}>
      <SystemContext value={services.system}>
        <CatalogContext value={services.catalog}>
          <MetadataContext value={services.metadata}>
            <MediaServersContext value={services.mediaServers}>
              <IngestionContext value={services.ingestion}>{children}</IngestionContext>
            </MediaServersContext>
          </MetadataContext>
        </CatalogContext>
      </SystemContext>
    </IdentityContext>
  )
}

/** Composition root: data source, adapters, query cache, ports and router. */
function ReadyApp({ services, history }: { services: Services; history?: RouterHistory }) {
  const [environment] = useState(() => createEnvironment(services, history))
  return (
    <InputProvider>
      <QueryClientProvider client={environment.queryClient}>
        <Ports services={services}>
          <Suspense fallback={<RouteLoading />}>
            <UploadUiProvider demo={dataSource === 'demo'}>
              <RouterProvider router={environment.router} />
            </UploadUiProvider>
          </Suspense>
        </Ports>
      </QueryClientProvider>
    </InputProvider>
  )
}

export function App({
  i18n: injected,
  services,
  history,
}: {
  i18n?: I18n
  services?: Services
  history?: RouterHistory
}) {
  const [instance] = useState(() => injected ?? createI18n())
  const [loadedServices, setLoadedServices] = useState<Services>()
  const selectedServices = services ?? loadedServices
  useEffect(() => {
    if (services || loadedServices) return
    let active = true
    void createServices(dataSource).then((loaded) => {
      if (active) setLoadedServices(loaded)
    })
    return () => {
      active = false
    }
  }, [services, loadedServices])
  return (
    <I18nextProvider i18n={instance}>
      {selectedServices ? (
        <ReadyApp services={selectedServices} {...(history ? { history } : {})} />
      ) : (
        <RouteLoading />
      )}
    </I18nextProvider>
  )
}
