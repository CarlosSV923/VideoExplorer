import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
    loadTitleMetadata: vi.fn(),
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
    const history = createMemoryHistory({ initialEntries: ['/movies'] })
    const expired = {
      ...services,
      identity: {
        ...services.identity,
        session: vi.fn(() => Promise.reject(new AppError('unauthorized'))),
      },
    }

    render(<App services={expired} history={history} />)

    await waitFor(() => {
      expect(history.location.pathname).toBe('/login')
    })
    expect(history.location.search).toContain('redirect=%2Fmovies')
  })

  it.each([
    ['/unassigned/7/assign', 'assign'],
    ['/movies/1', 'movie'],
    ['/series/2', 'series'],
    ['/movies/1/edit', 'edit'],
    ['/series/2/edit', 'seriesEdit'],
    ['/collections/9', 'collection'],
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
    ['/login', 'Entrar'],
    ['/movies?sort=year&genre=Drama', 'Películas'],
    ['/series?sort=added&incomplete=1', 'Series'],
    ['/search?q=dark', 'Buscar en la biblioteca'],
    ['/unassigned', 'No asignados'],
    ['/settings/general', 'Ajustes'],
    ['/settings/trash', 'Papelera'],
  ])('loads the English route and query names at %s', async (path, heading) => {
    render(
      <App
        i18n={createI18n('es')}
        services={routeServices}
        history={createMemoryHistory({ initialEntries: [path] })}
      />,
    )
    expect(await screen.findByRole('heading', { name: heading })).toBeInTheDocument()
  })

  it.each(['/peliculas', '/ajustes/papelera'])(
    'does not keep the former route %s and shows not found',
    async (path) => {
      render(
        <App
          i18n={createI18n('es')}
          services={routeServices}
          history={createMemoryHistory({ initialEntries: [path] })}
        />,
      )
      expect(
        await screen.findByRole('heading', { name: 'No encontramos esa página' }),
      ).toBeInTheDocument()
    },
  )

  it('writes the English browse search parameters when filters change', async () => {
    const catalog = {
      ...routeServices.catalog,
      libraryTitles: vi.fn(() =>
        Promise.resolve({
          titles: [
            {
              id: 1,
              kind: 'series' as const,
              title: 'Dark',
              discTitle: 'Dark',
              year: 2017,
              idTag: true,
              addedAt: '2026-10-01T00:00:00Z',
              genres: ['Drama', 'Comedia'],
              missingEpisodes: 1,
            },
          ],
        }),
      ),
    }
    const history = createMemoryHistory({ initialEntries: ['/series?sort=added&genre=Drama'] })
    render(
      <App i18n={createI18n('es')} services={{ ...routeServices, catalog }} history={history} />,
    )
    await waitFor(() => expect(screen.getByRole('group', { name: 'Ordenar' })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Año' }))
    await waitFor(() => {
      expect(history.location.search).toContain('sort=year')
    })
    fireEvent.click(screen.getByRole('button', { name: 'Drama' }))
    expect(screen.getByRole('checkbox', { name: 'Drama' })).toBeChecked()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Comedia' }))
    await waitFor(() => {
      expect(new URLSearchParams(history.location.search).get('genre')).toBe('Drama,Comedia')
    })
    fireEvent.click(screen.getByRole('button', { name: 'Quitar filtro' }))
    await waitFor(() => {
      expect(new URLSearchParams(history.location.search).get('genre')).toBeNull()
    })
    expect(screen.getByRole('checkbox', { name: 'Comedia' })).not.toBeChecked()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Comedia' }))
    await waitFor(() => {
      expect(new URLSearchParams(history.location.search).get('genre')).toBe('Comedia')
    })
    fireEvent.click(screen.getByRole('checkbox', { name: /Solo incomplet/ }))
    await waitFor(() => {
      expect(history.location.search).toContain('incomplete=1')
    })
  })

  it('reads and writes the English season parameter on a series detail route', async () => {
    const catalog = {
      ...routeServices.catalog,
      libraryTitle: vi.fn(() =>
        Promise.resolve({
          id: 2,
          kind: 'series' as const,
          title: 'Dark',
          discTitle: 'Dark',
          year: 2017,
          idTag: true,
          tmdbId: 1,
          originalTitle: 'Dark',
          overview: '',
          runtime: 0,
          genres: [],
          seasons: [
            { number: 1, episodes: [] },
            { number: 2, episodes: [] },
          ],
          videos: [],
        }),
      ),
    }
    const history = createMemoryHistory({ initialEntries: ['/series/2?season=1'] })
    render(
      <App i18n={createI18n('es')} services={{ ...routeServices, catalog }} history={history} />,
    )
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /Temporada 1/ })).toHaveAttribute(
        'aria-selected',
        'true',
      ),
    )
    fireEvent.click(screen.getByRole('tab', { name: /Temporada 2/ }))
    await waitFor(() => {
      expect(history.location.search).toContain('season=2')
    })
  })

  it.each([
    ['/movies/101', 'libraryTitle', 'Este título ya no está en la biblioteca', '/movies'],
    ['/series/102', 'libraryTitle', 'Este título ya no está en la biblioteca', '/series'],
    ['/collections/103', 'libraryCollection', 'Esta colección ya no está disponible', '/movies'],
    [
      '/unassigned/104/assign',
      'takeAssignLock',
      'Esta entrada ya no está en No asignados',
      '/unassigned',
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
        history={createMemoryHistory({ initialEntries: ['/movies/1'] })}
      />,
    )

    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: backLabel })).toHaveAttribute('href', '/movies')
    expect(screen.queryByRole('button', { name: /retry|reintentar/i })).not.toBeInTheDocument()
  })
})
