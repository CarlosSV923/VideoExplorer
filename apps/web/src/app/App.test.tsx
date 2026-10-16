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
})
