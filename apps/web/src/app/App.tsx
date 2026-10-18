import type { i18n as I18n } from 'i18next'
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, type RouterHistory } from '@tanstack/react-router'
import { useState, type ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'

import { CatalogContext } from '@/modules/catalog/application/ports'
import { IdentityContext } from '@/modules/identity/application/ports'
import { sessionKey } from '@/modules/identity/application/queries'
import { IngestionContext } from '@/modules/ingestion/application/ports'
import { MediaServersContext } from '@/modules/mediaservers/application/ports'
import { MetadataContext } from '@/modules/metadata/application/ports'
import { SystemContext } from '@/modules/system/application/ports'
import { UploadUiProvider } from '@/modules/ingestion/ui/UploadCenter'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { isAppError } from '@/shared/kernel/errors'

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
  const [selectedServices] = useState(() => services ?? createServices(dataSource))
  const [environment] = useState(() => createEnvironment(selectedServices, history))
  return (
    <I18nextProvider i18n={instance}>
      <InputProvider>
        <QueryClientProvider client={environment.queryClient}>
          <Ports services={selectedServices}>
            <UploadUiProvider demo={dataSource === 'demo'}>
              <RouterProvider router={environment.router} />
            </UploadUiProvider>
          </Ports>
        </QueryClientProvider>
      </InputProvider>
    </I18nextProvider>
  )
}
