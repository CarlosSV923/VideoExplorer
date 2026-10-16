import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'

import { IngestionContext, type IngestionPorts } from '@/modules/ingestion/application/ports'
import { MetadataContext } from '@/modules/metadata/application/ports'
import { createI18n } from '@/shared/i18n'

import { CatalogContext, type CatalogPorts } from '../application/ports'
import { matchesSearch, sortTitles } from '../application/search'
import { CatalogScreen } from './CatalogScreens'
import type { LibraryTitle, LibraryTitleSummary } from '../domain/types'

const summary = (overrides: Partial<LibraryTitleSummary> = {}): LibraryTitleSummary => ({
  id: 1,
  kind: 'movie',
  title: 'Película Ámbar',
  discTitle: 'Pelicula Ambar',
  year: 2024,
  idTag: true,
  addedAt: '2026-10-01T00:00:00Z',
  ...overrides,
})

const detail: LibraryTitle = {
  id: 1,
  kind: 'movie',
  title: 'Película Ámbar',
  discTitle: 'Pelicula Ambar',
  year: 2024,
  idTag: true,
  originalTitle: 'Amber Film',
  overview: 'Una película de prueba.',
  runtime: 120,
  genres: ['Drama'],
  backdrop: '/api/images/backdrop/amber.jpg',
  poster: '/api/images/poster/amber.jpg',
  seasons: [],
  videos: [{ id: 10, path: 'Película.mkv', size: 100, version: '', part: 0, subtitles: [] }],
}

function renderScreen(screenName: 'home' | 'edit', catalog: Partial<CatalogPorts>, metadata = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const ingestion = { jobs: vi.fn().mockResolvedValue([]) } as unknown as IngestionPorts
  const catalogPorts = {
    libraryTitles: vi.fn().mockResolvedValue({ titles: [summary()] }),
    libraryTitle: vi.fn().mockResolvedValue(detail),
    ...catalog,
  } as CatalogPorts
  const metadataPorts = {
    searchTitles: vi.fn().mockResolvedValue({ tmdbAvailable: true, library: [], tmdb: [] }),
    refreshTitleMetadata: vi.fn(),
    imageUrl: vi.fn((kind: string, file: string) => `/api/images/${kind}/${file}`),
    ...metadata,
  }
  return render(
    <I18nextProvider i18n={createI18n('es')}>
      <QueryClientProvider client={queryClient}>
        <CatalogContext value={catalogPorts}>
          <MetadataContext value={metadataPorts}>
            <IngestionContext value={ingestion}>
              <CatalogScreen screen={screenName} id={1} />
            </IngestionContext>
          </MetadataContext>
        </CatalogContext>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('Biblioteca', () => {
  beforeAll(() => {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }),
    })
  })

  it('searches without accents and orders by most recently added', () => {
    expect(matchesSearch(summary(), 'pelicula AMBAR')).toBe(true)
    expect(matchesSearch(summary(), 'película ciencia')).toBe(false)
    const titles = [
      summary({ id: 2, title: 'B', addedAt: '2026-10-02T00:00:00Z' }),
      summary({ id: 3, title: 'A', addedAt: '2026-10-03T00:00:00Z' }),
    ]
    expect(sortTitles(titles, 'added').map((title) => title.id)).toEqual([3, 2])
    expect(sortTitles(titles, 'title').map((title) => title.id)).toEqual([3, 2])
  })

  it('shows the rotating hero and exposes a pause control', async () => {
    renderScreen('home', {
      libraryTitles: vi.fn().mockResolvedValue({
        titles: [
          summary({ backdrop: '/backdrop.jpg' }),
          summary({ id: 2, title: 'Otra', backdrop: '/backdrop-2.jpg' }),
        ],
      }),
    })
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Película Ámbar' })).toBeInTheDocument(),
    )
    const pause = screen.getByRole('button', { name: 'Pausar destacados' })
    fireEvent.click(pause)
    expect(screen.getByRole('button', { name: 'Reanudar destacados' })).toBeInTheDocument()
  })

  it('previews and commits an edit through the injected ports', async () => {
    const plan = {
      titleId: 1,
      revision: 'a'.repeat(64),
      valid: true,
      kind: 'movie' as const,
      title: 'Película Ámbar',
      folder: 'Movies/Pelicula Ambar (2024)',
      videos: [{ fileId: 10, file: 'Película.mkv', changed: true, duplicates: [], allowed: [] }],
      subtitles: [],
      errors: [],
      warnings: [],
    }
    const catalog = {
      planTitleEdit: vi.fn().mockResolvedValue(plan),
      commitTitleEdit: vi
        .fn()
        .mockResolvedValue({ titleId: 1, kind: 'movie', changed: true, moves: [], warnings: [] }),
    }
    const metadata = {
      searchTitles: vi.fn().mockResolvedValue({
        tmdbAvailable: true,
        library: [],
        tmdb: [{ tmdbId: 1726, kind: 'movie' as const, title: 'Iron Man', year: 2008 }],
      }),
    }
    renderScreen('edit', catalog, metadata)
    await waitFor(() =>
      expect(screen.getByRole('textbox', { name: 'Coincidencia de TMDB' })).toBeInTheDocument(),
    )
    fireEvent.change(screen.getByRole('textbox', { name: 'Coincidencia de TMDB' }), {
      target: { value: 'Iron' },
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Iron Man/ })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: /Iron Man/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() =>
      expect(screen.getByText('Movies/Pelicula Ambar (2024)')).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))
    await waitFor(() => {
      expect(catalog.commitTitleEdit).toHaveBeenCalled()
    })
  })
})
