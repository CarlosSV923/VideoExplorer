import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'

import { CatalogContext, type CatalogPorts } from '@/modules/catalog/application/ports'
import { IdentityContext, type IdentityPorts } from '@/modules/identity/application/ports'
import { IngestionContext, type IngestionPorts } from '@/modules/ingestion/application/ports'
import {
  MediaServersContext,
  type MediaServersPorts,
} from '@/modules/mediaservers/application/ports'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { SystemContext, type SystemPorts } from '../application/ports'
import { SettingsScreen } from './SettingsScreen'
import { SystemHealthBanner } from './SystemHealthBanner'

function renderWith(
  ui: ReactNode,
  overrides: { system?: Partial<SystemPorts>; media?: Partial<MediaServersPorts> } = {},
) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const catalog = {
    scans: vi.fn().mockResolvedValue({ scans: [] }),
    runScans: vi.fn().mockResolvedValue({ scans: [] }),
  } as unknown as CatalogPorts
  const ingestion = {
    jobs: vi.fn().mockResolvedValue([]),
    watchJobs: vi.fn(() => () => undefined),
  } as unknown as IngestionPorts
  const media = {
    settings: vi.fn().mockResolvedValue({
      jellyfin: {
        state: 'connected',
        translationSource: 'detected',
        translation: {
          libraryPath: '/media',
          movies: '/media/Movies',
          series: '/media/TV Shows',
        },
        lastNotificationAt: '2026-10-16T00:00:00Z',
      },
    }),
    ...overrides.media,
  }
  const system = {
    health: vi.fn().mockResolvedValue({
      status: 'degraded',
      checks: [{ name: 'library', ok: false, code: 'library_not_writable', message: 'technical' }],
      jellyfin: { state: 'unconfigured' },
    }),
    ...overrides.system,
  }
  const identity = {
    session: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
  } as unknown as IdentityPorts
  return render(
    <I18nextProvider i18n={createI18n('es')}>
      <InputProvider>
        <QueryClientProvider client={client}>
          <IdentityContext value={identity}>
            <SystemContext value={system}>
              <MediaServersContext value={media}>
                <IngestionContext value={ingestion}>
                  <CatalogContext value={catalog}>{ui}</CatalogContext>
                </IngestionContext>
              </MediaServersContext>
            </SystemContext>
          </IdentityContext>
        </QueryClientProvider>
      </InputProvider>
    </I18nextProvider>,
  )
}

describe('system screens', () => {
  it('translates health codes and offers technical details', async () => {
    renderWith(<SystemHealthBanner />)
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('La biblioteca no permite escribir.'),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Detalles' }))
    expect(screen.getByText('Biblioteca no escribible')).toBeInTheDocument()
    expect(screen.getByText('technical')).toBeInTheDocument()
  })

  it('shows connected Jellyfin translation state', async () => {
    renderWith(<SettingsScreen />)
    await waitFor(() => expect(screen.getByText('Conectado')).toBeInTheDocument())
    expect(screen.getByText('Ruta detectada')).toBeInTheDocument()
    expect(screen.getByText('/media/Movies', { exact: false })).toBeInTheDocument()
  })
})
