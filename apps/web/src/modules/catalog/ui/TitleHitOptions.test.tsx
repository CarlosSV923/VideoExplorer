import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { MetadataContext, type MetadataPorts } from '@/modules/metadata/application/ports'
import type { ImageKind } from '@/modules/metadata/domain/types'
import type { TitleHit } from '../domain/types'
import { createI18n } from '@/shared/i18n'
import { InputProvider, useDispatch } from '@/shared/input'
import { TitleHitOptions } from './TitleHitOptions'

const library: TitleHit[] = Array.from({ length: 8 }, (_, index) => ({
  source: 'library',
  libraryTitleId: index + 1,
  tmdbId: index + 100,
  kind: 'movie',
  title: `Library Film ${String(index + 1)}`,
  year: 2000 + index,
  ...(index === 0 ? { posterFile: 'library-poster.jpg' } : {}),
}))
const tmdb: TitleHit[] = Array.from({ length: 8 }, (_, index) => ({
  source: 'tmdb',
  tmdbId: index + 200,
  kind: 'movie',
  title: `TMDB Film ${String(index + 1)}`,
  year: 2010 + index,
  posterFile: 'tmdb-poster.jpg',
}))

function Harness() {
  const [selected, setSelected] = useState<TitleHit | null>(null)
  const [query, setQuery] = useState('first')
  const metadata = {
    imageUrl: vi.fn((_kind: ImageKind, file: string) => `/api/images/poster/${file}`),
  } as unknown as MetadataPorts
  return (
    <I18nextProvider i18n={createI18n('es')}>
      <InputProvider>
        <MetadataContext value={metadata}>
          <label>
            Search
            <input
              value={query}
              onChange={(event) => {
                setQuery(event.currentTarget.value)
              }}
            />
          </label>
          <TitleHitOptions
            key={query}
            library={library}
            tmdb={tmdb}
            selected={selected}
            onPick={setSelected}
          />
        </MetadataContext>
      </InputProvider>
    </I18nextProvider>
  )
}

describe('TitleHitOptions', () => {
  it('renders selectable radios with poster, fallback, library chip and selection state', () => {
    render(<Harness />)
    const radio = screen.getByRole('radio', { name: /Library Film 1/ })
    expect(radio).toHaveAttribute('aria-checked', 'false')
    expect(radio).toHaveAttribute('tabindex', '0')
    const poster = radio.querySelector('img')
    expect(poster).toHaveAttribute('src', '/api/images/poster/library-poster.jpg')
    expect(poster).toHaveAttribute('loading', 'lazy')
    expect(within(radio).getByText('En tu biblioteca')).toBeInTheDocument()
    expect(screen.getByText('TMDB Film 1')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /TMDB Film 1/ })).toHaveAttribute(
      'aria-checked',
      'false',
    )
    expect(
      screen.getByRole('radiogroup', {
        name: 'Elige una coincidencia En tu biblioteca · Asignar a existente',
      }),
    ).toBeInTheDocument()
  })

  it('keeps one choice across both sections and supports arrow navigation and confirmation', async () => {
    render(<Harness />)
    const user = userEvent.setup()
    const first = screen.getByRole('radio', { name: /Library Film 1/ })
    const secondSection = screen.getByRole('radio', { name: /TMDB Film 1/ })
    act(() => {
      first.focus()
    })
    await user.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}')
    expect(secondSection).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(secondSection).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByText('Elegida')).toBeInTheDocument()
    expect(first).toHaveAttribute('aria-checked', 'false')
  })

  it('shows five per section, reveals five more, and resets the count when the search changes', () => {
    render(<Harness />)
    expect(screen.getAllByRole('radio')).toHaveLength(10)
    expect(screen.getAllByRole('button', { name: 'Mostrar 3 más' })).toHaveLength(2)
    const showMore = screen.getAllByRole('button', { name: 'Mostrar 3 más' })[0]
    if (!showMore) throw new Error('Expected a show more control')
    fireEvent.click(showMore)
    expect(screen.getAllByRole('radio')).toHaveLength(13)
    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'second' },
    })
    expect(screen.getAllByRole('radio')).toHaveLength(10)
  })

  it('selects the focused option when the gamepad confirms with A', () => {
    function PadHarness() {
      const metadata = { imageUrl: vi.fn() } as unknown as MetadataPorts
      return (
        <I18nextProvider i18n={createI18n('es')}>
          <InputProvider>
            <MetadataContext value={metadata}>
              <PadContent />
            </MetadataContext>
          </InputProvider>
        </I18nextProvider>
      )
    }
    function PadContent() {
      const [selected, setSelected] = useState<TitleHit | null>(null)
      const dispatch = useDispatch()
      return (
        <>
          <button
            type="button"
            onClick={() => {
              screen.getByRole('radio', { name: /TMDB Film 1/ }).focus()
              dispatch('confirm')
            }}
          >
            Pad A
          </button>
          <TitleHitOptions library={library} tmdb={tmdb} selected={selected} onPick={setSelected} />
        </>
      )
    }
    render(<PadHarness />)
    fireEvent.click(screen.getByRole('button', { name: 'Pad A' }))
    expect(screen.getByRole('radio', { name: /TMDB Film 1/ })).toHaveAttribute(
      'aria-checked',
      'true',
    )
  })

  it('keeps a selected result visible even when it is below the first five', () => {
    const SelectBelowFirstPage = () => {
      const [selected, setSelected] = useState<TitleHit | null>(library[7] ?? null)
      const metadata = { imageUrl: vi.fn() } as unknown as MetadataPorts
      return (
        <I18nextProvider i18n={createI18n('es')}>
          <InputProvider>
            <MetadataContext value={metadata}>
              <TitleHitOptions
                library={library}
                tmdb={tmdb}
                selected={selected}
                onPick={setSelected}
              />
            </MetadataContext>
          </InputProvider>
        </I18nextProvider>
      )
    }
    render(<SelectBelowFirstPage />)
    expect(screen.getAllByRole('radio')).toHaveLength(13)
    expect(screen.getByRole('radio', { name: /Library Film 8/ })).toHaveAttribute(
      'aria-checked',
      'true',
    )
  })

  it('replaces an image that fails with the generated poster', () => {
    render(<Harness />)
    const image = screen.getByRole('radio', { name: /Library Film 1/ }).querySelector('img')
    if (!image) throw new Error('Expected a poster image')
    fireEvent.error(image)
    const radio = screen.getByRole('radio', { name: /Library Film 1/ })
    expect(within(radio).queryByRole('img')).not.toBeInTheDocument()
    expect(within(radio).getAllByText('Library Film 1')).toHaveLength(2)
  })
})
