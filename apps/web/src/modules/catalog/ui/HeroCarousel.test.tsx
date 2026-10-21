import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import { CatalogContext, type CatalogPorts } from '../application/ports'
import type { LibraryTitle, LibraryTitleSummary } from '../domain/types'
import { createI18n } from '@/shared/i18n'
import { HeroCarousel } from './HeroCarousel'

const summary = (id: number, title: string): LibraryTitleSummary => ({
  id,
  kind: 'movie',
  title,
  discTitle: title,
  year: 2024,
  idTag: true,
  addedAt: '2026-10-01T00:00:00Z',
  backdrop: `/backdrop-${String(id)}.jpg`,
})

const detail = (item: LibraryTitleSummary, overrides: Partial<LibraryTitle> = {}): LibraryTitle =>
  ({
    ...item,
    originalTitle: item.title,
    overview: '',
    runtime: 120,
    genres: ['Drama'],
    seasons: [],
    videos: [],
    ...overrides,
  }) as unknown as LibraryTitle

function renderHero(details: LibraryTitle[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const catalog = {
    libraryTitle: vi.fn((id: number) =>
      Promise.resolve(details.find((item) => item.id === id) as LibraryTitle),
    ),
  } as unknown as CatalogPorts
  const titles = details.map((item) => summary(item.id, item.title))

  return render(
    <I18nextProvider i18n={createI18n('es')}>
      <QueryClientProvider client={queryClient}>
        <CatalogContext value={catalog}>
          <HeroCarousel titles={titles} />
        </CatalogContext>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('HeroCarousel', () => {
  beforeAll(() => {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    })
  })

  it('keeps the same size classes for wide/tall logos, text titles and long overviews', async () => {
    const titles = [
      detail(summary(1, 'Logo ancho'), { logo: '/logo-wide.svg' }),
      detail(summary(2, 'Logo alto'), { logo: '/logo-tall.svg' }),
      detail(summary(3, 'Sin logo')),
      detail(summary(4, 'Los juegos del hambre: Sinsajo - Parte 1')),
      detail(summary(5, 'Sinopsis larga'), {
        overview:
          'Una sinopsis deliberadamente larga para comprobar que el contenido se limita a tres líneas y no desplaza los controles del hero.',
      }),
      detail(summary(6, 'La vida secreta de Walter Mitty')),
    ]
    renderHero(titles)

    const hero = await screen.findByRole('region', { name: 'Destacados' })
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Logo ancho' })).toBeInTheDocument(),
    )

    const sizeClasses = () => ({
      hero: hero.className,
      title: hero.querySelector('h2')?.className,
      titleText: hero.querySelector('h2 span')?.className,
      metadata: hero.querySelector('p.text-body-sm')?.className,
      overview: hero.querySelector('p.text-body-lg')?.className,
      controls: hero.querySelector('div.h-11')?.className,
    })
    const classes = [sizeClasses()]

    for (let index = 1; index < titles.length; index += 1) {
      const title = titles[index]
      if (!title) continue
      fireEvent.click(screen.getByRole('button', { name: `Ir al destacado ${String(index + 1)}` }))
      await waitFor(() =>
        expect(screen.getByRole('heading', { name: title.title })).toBeInTheDocument(),
      )
      classes.push(sizeClasses())
    }

    expect(new Set(classes.map((value) => value.hero)).size).toBe(1)
    expect(new Set(classes.map((value) => value.title)).size).toBe(1)
    expect(new Set(classes.map((value) => value.metadata)).size).toBe(1)
    expect(new Set(classes.map((value) => value.overview)).size).toBe(1)
    expect(new Set(classes.map((value) => value.controls)).size).toBe(1)
    expect(hero).toHaveClass('h-[440px]', 'md:h-[600px]')
    expect(hero.querySelector('h2')).toHaveClass(
      'h-[110px]',
      'w-[min(300px,70vw)]',
      'md:h-40',
      'md:w-[min(520px,70vw)]',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Ir al destacado 4' }))
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Los juegos del hambre: Sinsajo - Parte 1' }),
      ).toBeInTheDocument(),
    )
    const longTitle = screen.getByRole('heading', {
      name: 'Los juegos del hambre: Sinsajo - Parte 1',
    })
    expect(longTitle).toHaveAttribute('title', 'Los juegos del hambre: Sinsajo - Parte 1')
    expect(longTitle.querySelector('span')).toHaveClass(
      'text-[calc(var(--text-display-hero)*0.55)]',
      'leading-[0.9]',
      'line-clamp-3',
    )
    expect(longTitle.querySelector('span')).toHaveTextContent(
      'Los juegos del hambre: Sinsajo - Parte 1',
    )
    expect(screen.queryByText('Destacado')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Ir al destacado 6' }))
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'La vida secreta de Walter Mitty' }),
      ).toBeInTheDocument(),
    )
    expect(
      screen
        .getByRole('heading', { name: 'La vida secreta de Walter Mitty' })
        .querySelector('span'),
    ).toHaveClass('text-[calc(var(--text-display-hero)*0.75)]')
    fireEvent.click(screen.getByRole('button', { name: 'Ir al destacado 3' }))
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Sin logo' })).toBeInTheDocument(),
    )
    expect(screen.getByRole('heading', { name: 'Sin logo' }).querySelector('span')).toHaveClass(
      'text-display-hero',
    )
    expect(hero.querySelector('p.text-body-lg')).toHaveClass('h-[81px]', 'line-clamp-3')
  })
})
