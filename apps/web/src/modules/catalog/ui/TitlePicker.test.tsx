import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { MetadataContext, type MetadataPorts } from '@/modules/metadata/application/ports'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { TitlePicker } from './TitlePicker'

describe('TitlePicker', () => {
  it('shows identifier help and a translated empty state when a search finds no title', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const searchTitles = vi.fn().mockResolvedValue({ tmdbAvailable: true, library: [], tmdb: [] })
    const metadata = {
      searchTitles,
      imageUrl: vi.fn(),
    } as unknown as MetadataPorts

    render(
      <I18nextProvider i18n={createI18n('es')}>
        <QueryClientProvider client={queryClient}>
          <InputProvider>
            <MetadataContext value={metadata}>
              <TitlePicker onChange={vi.fn()} />
            </MetadataContext>
          </InputProvider>
        </QueryClientProvider>
      </I18nextProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Elegir título' }))
    expect(screen.getByText('Busca por nombre, id de TMDB o id de IMDb.')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: 'Buscar título' }), {
      target: { value: '273477' },
    })

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('No se encontraron títulos.')
    })
    expect(searchTitles).toHaveBeenCalledWith('273477', undefined)
  })
})
