import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { IngestionContext, type IngestionPorts } from '@/modules/ingestion/application/ports'
import { MetadataContext, type MetadataPorts } from '@/modules/metadata/application/ports'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { AppError } from '@/shared/kernel/errors'
import type { CatalogPorts } from '../application/ports'
import { CatalogContext } from '../application/ports'
import type { AssignPlan, AssignSuggestions, UnassignedSection } from '../domain/types'
import { CatalogScreen } from './CatalogScreens'

const suggestions: AssignSuggestions = {
  entryId: 7,
  name: 'Dark.S01',
  relation: 'episodes',
  tmdbConfigured: true,
  title: { source: 'tmdb', tmdbId: 1, kind: 'series', title: 'Dark', year: 2017 },
  videos: [
    {
      file: 'Dark.S01E01.mkv',
      reading: { type: 'series', title: 'Dark', year: 2017, season: 1, chapter: 1 },
      match: { source: 'tmdb', tmdbId: 1, kind: 'series', title: 'Dark', year: 2017 },
    },
  ],
  subtitles: [
    {
      file: 'Dark.S01E01.es.srt',
      video: 'Dark.S01E01.mkv',
      language: 'es-419',
      forced: false,
      sdh: false,
    },
  ],
}

const section: UnassignedSection = {
  entries: [
    {
      id: 1,
      name: 'Dark.S01',
      origin: 'samba',
      relation: 'episodes',
      loose: false,
      status: 'ready',
      size: 100,
      files: [{ path: 'Dark.S01E01.mkv', size: 100, copying: false }],
    },
    {
      id: 2,
      name: 'Copiando',
      origin: 'samba',
      relation: 'none',
      loose: false,
      status: 'copying',
      size: 100,
      files: [{ path: 'copy.mkv', size: 100, copying: true }],
    },
    {
      id: 3,
      name: 'Oculta',
      origin: 'samba',
      relation: 'none',
      loose: false,
      status: 'ready',
      size: 0,
      files: [],
    },
  ],
  jobs: [],
}

function mockCalls(ports: CatalogPorts, name: keyof CatalogPorts) {
  const candidate = (ports as unknown as Record<string, unknown>)[name]
  if (typeof candidate !== 'function' || !('mock' in candidate)) return [] as unknown[][]
  return (candidate as unknown as { mock: { calls: unknown[][] } }).mock.calls
}

function renderScreen(screenName: 'unassigned' | 'assign', catalog: Partial<CatalogPorts> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const ports = {
    unassigned: vi.fn().mockResolvedValue(section),
    takeAssignLock: vi.fn().mockResolvedValue({ token: 'lock', expiresAt: '2099-01-01T00:00:00Z' }),
    releaseAssignLock: vi.fn().mockResolvedValue(undefined),
    assignSuggestions: vi.fn().mockResolvedValue(suggestions),
    planAssign: vi.fn().mockResolvedValue({
      entryId: 7,
      valid: true,
      videos: [
        {
          file: 'Dark.S01E01.mkv',
          keep: false,
          finalPath: 'TV Shows/Dark/S01E01.mkv',
          duplicates: [],
          exactCopy: false,
          allowed: [],
        },
      ],
      subtitles: [
        {
          file: 'Dark.S01E01.es.srt',
          video: 'Dark.S01E01.mkv',
          finalPath: 'TV Shows/Dark/S01E01.es.srt',
        },
      ],
      errors: [],
      warnings: [],
    } satisfies AssignPlan),
    commitAssign: vi.fn().mockResolvedValue({ entryId: 7, entryRemoved: true, placed: [] }),
    downloadUnassignedEntryUrl: vi.fn().mockReturnValue('/download/entry'),
    downloadUnassignedFileUrl: vi.fn().mockReturnValue('/download/file'),
    trashUnassignedEntry: vi.fn().mockResolvedValue(undefined),
    deleteUnassignedEntry: vi.fn().mockResolvedValue(undefined),
    trashUnassignedFile: vi.fn().mockResolvedValue(undefined),
    deleteUnassignedFile: vi.fn().mockResolvedValue(undefined),
    ...catalog,
  } as CatalogPorts
  const metadata = {
    searchTitles: vi.fn().mockResolvedValue({ tmdbAvailable: true, library: [], tmdb: [] }),
    imageUrl: vi.fn(),
  } as unknown as MetadataPorts
  const ingestion = { jobs: vi.fn().mockResolvedValue([]) } as unknown as IngestionPorts
  const view = render(
    <I18nextProvider i18n={createI18n('es')}>
      <QueryClientProvider client={queryClient}>
        <InputProvider>
          <CatalogContext value={ports}>
            <MetadataContext value={metadata}>
              <IngestionContext value={ingestion}>
                <CatalogScreen screen={screenName} id={7} />
              </IngestionContext>
            </MetadataContext>
          </CatalogContext>
        </InputProvider>
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return { ...view, ports }
}

describe('No asignados', () => {
  it('renders ready, copying and hidden-only entries and confirms permanent deletion', async () => {
    const { ports } = renderScreen('unassigned')
    await waitFor(() => {
      expect(screen.getAllByText('Lista para asignar').length).toBeGreaterThan(0)
    })
    expect(screen.getByText('Copiando…')).toBeInTheDocument()
    expect(screen.getByText('Sin archivos visibles')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Más acciones de Dark.S01' }))
    fireEvent.click(
      within(screen.getByRole('menu')).getByRole('menuitem', { name: 'Borrar definitivamente' }),
    )
    expect(mockCalls(ports, 'deleteUnassignedEntry')).toHaveLength(0)
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Borrar definitivamente' }),
    )
    await waitFor(() => {
      expect(mockCalls(ports, 'deleteUnassignedEntry')).toContainEqual([1])
    })
  })
})

describe('Asignar', () => {
  it('shows the keyboard groups and saves with Ctrl+Enter after preview', async () => {
    const { ports } = renderScreen('assign')
    await waitFor(() => expect(screen.getByText('Dark.S01E01.mkv')).toBeInTheDocument())
    expect(screen.getByText(/Con el foco en un video/)).toBeInTheDocument()
    expect(screen.getByText('Siempre')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => {
      expect(mockCalls(ports, 'planAssign').length).toBeGreaterThan(0)
    })
    fireEvent.keyDown(document, { key: 'Enter', ctrlKey: true })
    await waitFor(() => {
      expect(mockCalls(ports, 'commitAssign').length).toBe(1)
    })
  })

  it('takes the lock, renews it on preview and releases it on cancel', async () => {
    const { ports } = renderScreen('assign')
    await waitFor(() => expect(screen.getByText('Dark.S01E01.mkv')).toBeInTheDocument())
    expect(mockCalls(ports, 'takeAssignLock')).toContainEqual([7])
    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => {
      expect(mockCalls(ports, 'planAssign').length).toBeGreaterThan(0)
    })
    fireEvent.click(screen.getByRole('button', { name: 'Anterior' }))
    await waitFor(() => {
      expect(mockCalls(ports, 'releaseAssignLock')).toContainEqual([7, 'lock'])
    })
  })

  it('releases the lock when leaving the screen and on pagehide with keepalive', async () => {
    const { ports, unmount } = renderScreen('assign')
    await waitFor(() => expect(screen.getByText('Dark.S01E01.mkv')).toBeInTheDocument())

    fireEvent(window, new Event('pagehide'))
    expect(mockCalls(ports, 'releaseAssignLock')).toContainEqual([
      7,
      'lock',
      { keepalive: true },
    ])
    unmount()
    expect(mockCalls(ports, 'releaseAssignLock')).toHaveLength(1)
  })

  it('translates lock_lost and plan_changed conflicts', async () => {
    renderScreen('assign', {
      planAssign: vi.fn().mockRejectedValue(new AppError('conflict', { code: 'lock_lost' })),
    })
    await waitFor(() => expect(screen.getByText('Dark.S01E01.mkv')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() =>
      expect(screen.getByText(/se liberó por falta de actividad/)).toBeInTheDocument(),
    )

    cleanup()
    const second = renderScreen('assign', {
      planAssign: vi
        .fn()
        .mockResolvedValueOnce({
          ...({
            entryId: 7,
            valid: true,
            videos: [
              {
                file: 'Dark.S01E01.mkv',
                keep: false,
                finalPath: 'TV Shows/Dark/S01E01.mkv',
                duplicates: [],
                exactCopy: false,
                allowed: [],
              },
            ],
            subtitles: [],
            errors: [],
            warnings: [],
          } satisfies AssignPlan),
        })
        .mockRejectedValueOnce(new AppError('conflict', { code: 'plan_changed' })),
    })
    await waitFor(() => {
      expect(
        within(second.container).getByRole('button', { name: 'Previsualizar' }),
      ).toBeInTheDocument()
    })
    fireEvent.click(within(second.container).getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => {
      expect(mockCalls(second.ports, 'planAssign').length).toBeGreaterThan(0)
    })
    fireEvent.click(within(second.container).getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => {
      expect(within(second.container).getByText(/destino cambió/)).toBeInTheDocument()
    })
  })

  it('blocks save on undecided duplicates and disallows keep both for parts', async () => {
    const { ports } = renderScreen('assign', {
      planAssign: vi.fn().mockResolvedValue({
        entryId: 7,
        valid: false,
        videos: [
          {
            file: 'Dark.S01E01.mkv',
            keep: false,
            duplicates: [{ rel: 'TV Shows/Dark/existing.mkv', size: 100 }],
            exactCopy: true,
            allowed: ['replace', 'skip'],
            proposal: 'skip',
          },
        ],
        subtitles: [],
        errors: [{ file: 'Dark.S01E01.mkv', field: 'duplicate', code: 'duplicate' }],
        warnings: [],
      } satisfies AssignPlan),
    })
    await waitFor(() => expect(screen.getByText('Dark.S01E01.mkv')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => {
      expect(screen.getAllByText('Copia exacta').length).toBeGreaterThan(0)
    })
    expect(
      screen.getAllByText('Decide todos los duplicados para poder guardar.').length,
    ).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: 'Conservar ambos' })).not.toBeInTheDocument()
    expect(mockCalls(ports, 'commitAssign')).toHaveLength(0)
  })

  it('accepts a free title and letters only work on the video card, not a field', async () => {
    const { ports } = renderScreen('assign')
    await waitFor(() => expect(screen.getByText('Dark.S01E01.mkv')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Título de TMDB' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Título' }), {
      target: { value: 'Mi serie' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Usar nombre libre' }))
    const card = screen.getByText('Dark.S01E01.mkv').closest('article')
    expect(card).not.toBeNull()
    if (card) {
      card.focus()
      fireEvent.keyDown(card, { key: 'R' })
    }
    const chapter = screen.getByRole('spinbutton', { name: 'Capítulo inicial' })
    fireEvent.keyDown(chapter, { key: 'A' })
    fireEvent.click(screen.getByRole('button', { name: 'Previsualizar' }))
    await waitFor(() => {
      expect(mockCalls(ports, 'planAssign').length).toBeGreaterThan(0)
    })
    const request = mockCalls(ports, 'planAssign')[0]?.[2] as
      { videos?: Array<{ title?: unknown; duplicate?: unknown }> } | undefined
    expect(request?.videos?.[0]?.title).toMatchObject({ source: 'free', title: 'Mi serie' })
    expect(request?.videos?.[0]?.duplicate).toBe('replace')
  })
})
