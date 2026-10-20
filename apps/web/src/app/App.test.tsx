import { render, screen, waitFor } from '@testing-library/react'
import { createMemoryHistory } from '@tanstack/react-router'

import { createI18n, type Language } from '@/shared/i18n'
import { AppError } from '@/shared/kernel/errors'

import { App } from './App'
import type { Services } from './services'

const services = {
  identity: {
    session: vi.fn(() => Promise.resolve({ expiresAt: '2099-01-01T00:00:00Z' })),
    login: vi.fn(() => Promise.resolve()),
    logout: vi.fn(() => Promise.resolve()),
  },
  catalog: {},
  ingestion: {},
  mediaServers: {},
  metadata: {},
  system: {},
} as unknown as Services

const routeServices = {
  identity: {
    session: vi.fn(() => Promise.resolve({ expiresAt: '2099-01-01T00:00:00Z' })),
    login: vi.fn(() => Promise.resolve()),
    logout: vi.fn(() => Promise.resolve()),
  },
  catalog: {
    libraryTitles: vi.fn(() => Promise.resolve({ titles: [] })),
    libraryTitle: vi.fn(() =>
      Promise.resolve({
        id: 1,
        kind: 'movie',
        title: 'Alien Romulus',
        discTitle: 'Alien Romulus',
        year: 2024,
        idTag: true,
        tmdbId: 945961,
        originalTitle: 'Alien: Romulus',
        overview: '',
        runtime: 119,
        genres: [],
        seasons: [],
        videos: [],
      }),
    ),
    libraryCollection: vi.fn(() =>
      Promise.resolve({
        id: 9,
        name: 'Alien',
        overview: '',
        movies: [],
      }),
    ),
    planTitleEdit: vi.fn(),
    commitTitleEdit: vi.fn(),
    takeAssignLock: vi.fn(() =>
      Promise.resolve({ token: 'route-lock', expiresAt: '2099-01-01T00:00:00Z' }),
    ),
    releaseAssignLock: vi.fn(() => Promise.resolve()),
    assignSuggestions: vi.fn(() =>
      Promise.resolve({
        entryId: 7,
        name: 'Dark.S01',
        relation: 'episodes',
        tmdbConfigured: true,
        title: { source: 'tmdb', tmdbId: 1, kind: 'series', title: 'Dark', year: 2017 },
        videos: [
          {
            file: 'Dark.S01E01.mkv',
            reading: { type: 'series', title: 'Dark', season: 1, chapter: 1 },
          },
        ],
        subtitles: [],
      }),
    ),
    unassigned: vi.fn(() => Promise.resolve({ entries: [], jobs: [] })),
    downloadUnassignedEntryUrl: vi.fn(() => '/download'),
    downloadUnassignedFileUrl: vi.fn(() => '/download'),
  },
  ingestion: {
    jobs: vi.fn(() => Promise.resolve([])),
    watchJobs: vi.fn(() => () => undefined),
  },
  metadata: {
    searchTitles: vi.fn(() => Promise.resolve({ tmdbAvailable: true, library: [], tmdb: [] })),
    refreshTitleMetadata: vi.fn(),
  },
  mediaServers: {},
  system: {},
} as unknown as Services

describe('App', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it.each<[Language, string]>([
    ['es', 'Tu biblioteca de películas y series en el NAS'],
    ['en', 'Your movie and series library on the NAS'],
  ])('renders the home route in %s', async (language, tagline) => {
    render(
      <App
        i18n={createI18n(language)}
        services={services}
        history={createMemoryHistory({ initialEntries: ['/'] })}
      />,
    )

    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
        language === 'es' ? 'Inicio' : 'Home',
      ),
    )
    expect(
      screen.getByText(
        language === 'es'
          ? 'La pantalla de inicio estará disponible aquí.'
          : 'The home screen will be available here.',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByText(tagline)).not.toBeInTheDocument()
  })

  it('defaults to Spanish when no language was chosen', async () => {
    render(<App services={services} history={createMemoryHistory({ initialEntries: ['/'] })} />)

    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Inicio'),
    )
  })

  it('redirects an expired session to login with the current route', async () => {
    const history = createMemoryHistory({ initialEntries: ['/peliculas'] })
    const expired = {
      ...services,
      identity: {
        ...services.identity,
        session: vi.fn(() => Promise.reject(new AppError('unauthorized'))),
      },
    }

    render(<App services={expired} history={history} />)

    await waitFor(() => {
      expect(history.location.pathname).toBe('/entrar')
    })
    expect(history.location.search).toContain('redirect=%2Fpeliculas')
  })

  it.each([
    ['/no-asignados/7/asignar', 'assign'],
    ['/peliculas/1', 'movie'],
    ['/series/2', 'series'],
    ['/peliculas/1/editar', 'edit'],
    ['/series/2/editar', 'seriesEdit'],
    ['/colecciones/9', 'collection'],
  ])('resuelve la ruta real %s sin caer en el estado vacío', async (path, route) => {
    const history = createMemoryHistory({ initialEntries: [path] })
    render(<App i18n={createI18n('es')} services={routeServices} history={history} />)

    if (route === 'assign') {
      await waitFor(() => {
        const calls = (
          routeServices.catalog as unknown as {
            takeAssignLock: { mock: { calls: unknown[][] } }
          }
        ).takeAssignLock.mock.calls
        expect(calls).toContainEqual([7])
      })
      expect(await screen.findByText('Dark.S01E01.mkv')).toBeInTheDocument()
    } else if (route === 'movie' || route === 'series') {
      expect(await screen.findByRole('heading', { name: 'Alien Romulus' })).toBeInTheDocument()
    } else if (route === 'edit' || route === 'seriesEdit') {
      expect(await screen.findByRole('heading', { name: 'Editar y reasignar' })).toBeInTheDocument()
    } else {
      expect(await screen.findByRole('heading', { name: 'Alien' })).toBeInTheDocument()
    }
    expect(
      screen.queryByText('La ruta no incluye los datos necesarios para mostrarla.'),
    ).not.toBeInTheDocument()
  })

  it.each([
    ['/peliculas/101', 'libraryTitle', 'Este título ya no está en la biblioteca', '/peliculas'],
    ['/series/102', 'libraryTitle', 'Este título ya no está en la biblioteca', '/series'],
    ['/colecciones/103', 'libraryCollection', 'Esta colección ya no está disponible', '/peliculas'],
    [
      '/no-asignados/104/asignar',
      'takeAssignLock',
      'Esta entrada ya no está en No asignados',
      '/no-asignados',
    ],
  ])('shows a missing resource state for %s', async (path, port, title, backHref) => {
    const catalog = {
      ...routeServices.catalog,
      [port]: vi.fn(() => Promise.reject(new AppError('notFound', { status: 404 }))),
    }
    const missingServices = { ...routeServices, catalog }
    render(
      <App
        i18n={createI18n('es')}
        services={missingServices}
        history={createMemoryHistory({ initialEntries: [path] })}
      />,
    )

    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Volver/ })).toHaveAttribute('href', backHref)
    expect(screen.queryByRole('button', { name: 'Reintentar' })).not.toBeInTheDocument()
  })

  it.each([
    ['es', 'Este título ya no está en la biblioteca', 'Volver a Películas'],
    ['en', 'This title is no longer in the library', 'Back to Movies'],
  ] as const)('translates the missing movie state in %s', async (language, title, backLabel) => {
    const missingServices = {
      ...routeServices,
      catalog: {
        ...routeServices.catalog,
        libraryTitle: vi.fn(() => Promise.reject(new AppError('notFound', { status: 404 }))),
      },
    } as unknown as Services
    render(
      <App
        i18n={createI18n(language)}
        services={missingServices}
        history={createMemoryHistory({ initialEntries: ['/peliculas/1'] })}
      />,
    )

    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: backLabel })).toHaveAttribute('href', '/peliculas')
    expect(screen.queryByRole('button', { name: /retry|reintentar/i })).not.toBeInTheDocument()
  })
})
