import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { IngestionContext, type IngestionPorts } from '@/modules/ingestion/application/ports'
import { MetadataContext, type MetadataPorts } from '@/modules/metadata/application/ports'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { AppError } from '@/shared/kernel/errors'
import type { LibraryTitle, LibraryTitleSummary } from '../domain/types'
import { CatalogContext, type CatalogPorts } from '../application/ports'
import { CatalogScreen } from './CatalogScreens'

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

const movie = {
  ...summary(),
  originalTitle: 'Amber Film',
  overview: 'Una película de prueba.',
  runtime: 120,
  genres: ['Drama'],
  seasons: [],
  videos: [{ id: 10, path: 'Película.mkv', size: 100, version: '', part: 0, subtitles: [] }],
} as unknown as LibraryTitle

const series = {
  ...summary({ id: 2, kind: 'series', title: 'Serie Ámbar', missingEpisodes: 2 }),
  originalTitle: 'Amber Series',
  overview: 'Una serie de prueba.',
  runtime: 0,
  genres: ['Drama'],
  seasons: [
    {
      number: 1,
      episodes: [
        {
          id: 101,
          number: 1,
          title: 'Piloto',
          overview: 'Primero',
          status: 'present',
          files: [
            {
              id: 11,
              path: 'S01E01.mkv',
              size: 100,
              version: '',
              part: 0,
              season: 1,
              chapter: 1,
              lastChapter: 1,
              subtitles: [],
            },
          ],
        },
        { id: 102, number: 2, title: 'Segundo', overview: 'Falta', status: 'missing', files: [] },
        {
          id: 103,
          number: 3,
          title: 'Tercero',
          overview: 'Después',
          status: 'upcoming',
          files: [],
        },
      ],
    },
  ],
  videos: [
    {
      id: 11,
      path: 'S01E01.mkv',
      size: 100,
      version: '',
      part: 0,
      season: 1,
      chapter: 1,
      lastChapter: 1,
      subtitles: [],
    },
  ],
} as unknown as LibraryTitle

function renderCatalog(
  screenName: string,
  overrides: Partial<CatalogPorts> = {},
  options: {
    id?: number
    search?: { orden?: string; genero?: string; incompletas?: string; q?: string }
  } = {},
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const catalog = {
    libraryTitles: vi.fn().mockResolvedValue({ titles: [summary()] }),
    libraryTitle: vi.fn().mockResolvedValue(movie),
    libraryCollection: vi
      .fn()
      .mockResolvedValue({ id: 9, name: 'Colección', overview: '', movies: [] }),
    moveTitleToUnassigned: vi.fn().mockResolvedValue(undefined),
    trashTitleScope: vi.fn().mockResolvedValue(undefined),
    downloadFileUrl: vi.fn().mockReturnValue('/download'),
    commitTitleEdit: vi
      .fn()
      .mockResolvedValue({ titleId: 1, kind: 'movie', changed: true, moves: [], warnings: [] }),
    planTitleEdit: vi.fn().mockResolvedValue({
      titleId: 1,
      revision: 'a'.repeat(64),
      valid: true,
      kind: 'movie',
      title: movie.title,
      folder: 'Movies/Pelicula',
      videos: [],
      subtitles: [],
      errors: [],
      warnings: [],
    }),
    ...overrides,
  } as unknown as CatalogPorts
  const metadata = {
    searchTitles: vi.fn().mockResolvedValue({ tmdbAvailable: true, library: [], tmdb: [] }),
    refreshTitleMetadata: vi.fn().mockResolvedValue(undefined),
    imageUrl: vi.fn((kind: string, file: string) => `/api/images/${kind}/${file}`),
  } as unknown as MetadataPorts
  const ingestion = { jobs: vi.fn().mockResolvedValue([]) } as unknown as IngestionPorts
  const result = render(
    <I18nextProvider i18n={createI18n('es')}>
      <QueryClientProvider client={queryClient}>
        <InputProvider>
          <CatalogContext value={catalog}>
            <MetadataContext value={metadata}>
              <IngestionContext value={ingestion}>
                <CatalogScreen screen={screenName} id={options.id ?? 1} search={options.search} />
              </IngestionContext>
            </MetadataContext>
          </CatalogContext>
        </InputProvider>
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return { ...result, catalog }
}

function mockCalls(catalog: CatalogPorts, name: keyof CatalogPorts) {
  const method = (catalog as unknown as Record<string, unknown>)[name]
  return typeof method === 'function'
    ? (method as unknown as { mock: { calls: unknown[][] } }).mock.calls
    : []
}

describe('Biblioteca: comportamiento de pantallas', () => {
  it('filters movies by genre and applies the selected sort', async () => {
    renderCatalog('movies', {
      libraryTitles: vi.fn().mockResolvedValue({
        titles: [
          summary({ id: 1, title: 'Zeta', genres: ['Drama'], addedAt: '2026-10-01T00:00:00Z' }),
          summary({ id: 2, title: 'Alfa', genres: ['Comedia'], addedAt: '2026-10-02T00:00:00Z' }),
        ],
      }),
    })
    await waitFor(() => expect(screen.getByRole('link', { name: /Zeta/ })).toBeInTheDocument())
    fireEvent.click(screen.getByLabelText('Comedia'))
    expect(screen.getByRole('link', { name: /Alfa/ })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Zeta/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Título' }))
    expect(screen.getByRole('link', { name: /Alfa/ })).toBeInTheDocument()
  })

  it('filters series with Solo incompletas', async () => {
    renderCatalog('series', {
      libraryTitles: vi.fn().mockResolvedValue({
        titles: [
          summary({ id: 2, kind: 'series', title: 'Incompleta', missingEpisodes: 1 }),
          summary({ id: 3, kind: 'series', title: 'Completa', missingEpisodes: 0 }),
        ],
      }),
    })
    await waitFor(() =>
      expect(screen.getByRole('link', { name: /Incompleta/ })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Solo incompletas' }))
    expect(screen.getByRole('link', { name: /Incompleta/ })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Completa/ })).not.toBeInTheDocument()
  })

  it('shows series seasons and episode status, with file actions only for present episodes', async () => {
    renderCatalog('seriesDetail', { libraryTitle: vi.fn().mockResolvedValue(series) }, { id: 2 })
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: 'Temporada 1' })).toBeInTheDocument(),
    )
    expect(screen.getByText('Presente')).toBeInTheDocument()
    expect(screen.getAllByText('Falta')).toHaveLength(2)
    expect(screen.getByText('Próximamente')).toBeInTheDocument()
    expect(screen.getAllByText('S01E01.mkv')).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Acciones del archivo S01E01.mkv' })).toHaveLength(
      1,
    )
  })

  it('dims only the poster for collection members outside the library', async () => {
    renderCatalog(
      'collection',
      {
        libraryCollection: vi.fn().mockResolvedValue({
          id: 9,
          name: 'Colección',
          overview: '',
          movies: [
            {
              id: 1,
              title: 'Dentro',
              originalTitle: 'Inside',
              year: 2020,
              inLibrary: true,
              poster: '/inside.jpg',
              libraryTitleId: 1,
            },
            {
              id: 2,
              title: 'Fuera',
              originalTitle: 'Outside',
              year: 2021,
              inLibrary: false,
              poster: '/outside.jpg',
            },
          ],
        }),
      },
      { id: 9 },
    )
    await waitFor(() => expect(screen.getByRole('article', { name: /Fuera/ })).toBeInTheDocument())
    const outside = screen.getByRole('article', { name: /Fuera/ })
    expect(outside.querySelector('img')).toHaveClass('brightness-55')
    expect(outside).not.toHaveClass('brightness-55')
  })

  it('asks for confirmation before sending a title to trash', async () => {
    const { catalog } = renderCatalog('movieDetail')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Mandar a la papelera' })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Mandar a la papelera' }))
    expect(mockCalls(catalog, 'trashTitleScope')).toHaveLength(0)
    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveTextContent('¿Mandar este título a la papelera?')
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Mandar a la papelera' }),
    )
    await waitFor(() => {
      expect(mockCalls(catalog, 'trashTitleScope')).toContainEqual([1, { kind: 'title' }])
    })
  })

  it('translates plan_changed when saving an edit', async () => {
    const { catalog } = renderCatalog('edit', {
      commitTitleEdit: vi
        .fn()
        .mockRejectedValue(new AppError('conflict', { code: 'plan_changed' })),
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Previsualizar' })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))
    await waitFor(() => expect(screen.getByText('El título cambió.')).toBeInTheDocument())
    expect(mockCalls(catalog, 'commitTitleEdit').length).toBeGreaterThan(0)
  })

  it('shows the empty home state', async () => {
    renderCatalog('home', { libraryTitles: vi.fn().mockResolvedValue({ titles: [] }) })
    await waitFor(() => expect(screen.getByText('Todavía no hay títulos')).toBeInTheDocument())
  })

  it('shows collection cards with library counts and collection links on home', async () => {
    renderCatalog('home', {
      libraryTitles: vi.fn().mockResolvedValue({
        titles: [
          summary({
            id: 1,
            title: 'Alien',
            collection: { id: 9, name: 'Alien: colección' },
          }),
          summary({
            id: 2,
            title: 'Aliens',
            collection: { id: 9, name: 'Alien: colección' },
          }),
        ],
      }),
      libraryCollection: vi.fn().mockResolvedValue({
        id: 9,
        name: 'Alien: colección',
        overview: '',
        movies: [
          { id: 1, title: 'Alien', originalTitle: 'Alien', year: 1979, inLibrary: true },
          { id: 2, title: 'Aliens', originalTitle: 'Aliens', year: 1986, inLibrary: false },
          { id: 3, title: 'Alien 3', originalTitle: 'Alien 3', year: 1992, inLibrary: false },
        ],
      }),
    })
    await waitFor(() =>
      expect(screen.getByRole('link', { name: /Alien: colección/ })).toBeInTheDocument(),
    )
    const card = screen.getByRole('link', { name: /Alien: colección/ })
    expect(card).toHaveAttribute('href', '/colecciones/9')
    await waitFor(() => expect(card).toHaveTextContent('1 de 3 en tu biblioteca'))
  })

  it('does not rotate the hero when reduced motion is preferred', async () => {
    const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({
        matches: true,
        media: '(prefers-reduced-motion: reduce)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }),
    })
    const interval = vi.spyOn(window, 'setInterval')
    try {
      renderCatalog('home', {
        libraryTitles: vi.fn().mockResolvedValue({
          titles: [
            summary({ title: 'Primero', backdrop: '/one.jpg' }),
            summary({ id: 2, title: 'Segundo', backdrop: '/two.jpg' }),
          ],
        }),
      })
      await waitFor(() =>
        expect(screen.getByRole('heading', { name: 'Primero' })).toBeInTheDocument(),
      )
      expect(interval.mock.calls.filter((call) => call[1] === 8000)).toHaveLength(0)
    } finally {
      interval.mockRestore()
      if (originalMatchMedia) Object.defineProperty(window, 'matchMedia', originalMatchMedia)
      else Object.defineProperty(window, 'matchMedia', { configurable: true, value: undefined })
    }
  })
})
