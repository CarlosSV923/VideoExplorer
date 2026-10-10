import { render, screen } from '@testing-library/react'

import { createI18n, type Language } from '@/shared/i18n'

import { App } from './App'

describe('App', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it.each<[Language, string]>([
    ['es', 'Tu biblioteca de películas y series en el NAS'],
    ['en', 'Your movie and series library on the NAS'],
  ])('renders the start screen in %s', (language, tagline) => {
    render(<App i18n={createI18n(language)} />)

    expect(screen.getByRole('heading', { level: 1, name: 'VideoExplorer' })).toBeInTheDocument()
    expect(screen.getByText(tagline)).toBeInTheDocument()
  })

  it('defaults to Spanish when no language was chosen', () => {
    render(<App />)

    expect(screen.getByText('Tu biblioteca de películas y series en el NAS')).toBeInTheDocument()
  })
})
