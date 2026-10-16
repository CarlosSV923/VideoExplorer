import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'

import { CatalogContext, type CatalogPorts } from '../application/ports'
import type { TrashRestorePlan } from '../domain/types'
import { IngestionContext, type IngestionPorts } from '@/modules/ingestion/application/ports'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { TrashScreen } from './TrashScreen'

const plan: TrashRestorePlan = {
  entryId: 4,
  files: [
    {
      file: 'film.mkv',
      destinations: ['Movies/Film/film.mkv'],
      duplicates: [{ rel: 'Movies/Film/film.mkv', size: 10 }],
      allowed: ['replace', 'keep_both', 'cancel'],
      proposal: 'keep_both',
      versionProposal: '1080p',
      versionIncomplete: false,
    },
  ],
}

function renderTrash(overrides: Partial<CatalogPorts> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const catalog = {
    libraryTitles: vi.fn(),
    libraryTitle: vi.fn(),
    libraryCollection: vi.fn(),
    fileTechnical: vi.fn(),
    downloadTitleUrl: vi.fn(),
    downloadSeasonUrl: vi.fn(),
    downloadFileUrl: vi.fn(),
    planTitleEdit: vi.fn(),
    commitTitleEdit: vi.fn(),
    planTitleIDTag: vi.fn(),
    commitTitleIDTag: vi.fn(),
    moveTitleToUnassigned: vi.fn(),
    trashTitleScope: vi.fn(),
    unassigned: vi.fn(),
    takeAssignLock: vi.fn(),
    releaseAssignLock: vi.fn(),
    assignSuggestions: vi.fn(),
    planAssign: vi.fn(),
    commitAssign: vi.fn(),
    trashUnassignedEntry: vi.fn(),
    deleteUnassignedEntry: vi.fn(),
    trashUnassignedFile: vi.fn(),
    deleteUnassignedFile: vi.fn(),
    downloadUnassignedEntryUrl: vi.fn(),
    downloadUnassignedFileUrl: vi.fn(),
    scans: vi.fn().mockResolvedValue({ scans: [] }),
    runScans: vi.fn(),
    trash: vi.fn().mockResolvedValue({
      entries: [
        {
          id: 4,
          origin: 'library',
          reason: 'removed',
          trashedAt: '2026-10-16T00:00:00Z',
          purgeAt: '2026-11-15T00:00:00Z',
          files: [
            { name: 'film.mkv', size: 10 },
            { name: 'film.es.srt', size: 2 },
          ],
        },
      ],
    }),
    emptyTrash: vi.fn().mockResolvedValue(undefined),
    deleteTrashEntry: vi.fn().mockResolvedValue(undefined),
    planTrashRestore: vi.fn().mockResolvedValue(plan),
    restoreTrashEntry: vi.fn().mockResolvedValue({ restored: ['Movies/Film/film.mkv'] }),
    ...overrides,
  }
  const ingestion = {
    jobs: vi.fn().mockResolvedValue([]),
    watchJobs: vi.fn(() => () => undefined),
  } as unknown as IngestionPorts
  return render(
    <I18nextProvider i18n={createI18n('es')}>
      <InputProvider>
        <QueryClientProvider client={client}>
          <IngestionContext value={ingestion}>
            <CatalogContext value={catalog}>
              <TrashScreen />
            </CatalogContext>
          </IngestionContext>
        </QueryClientProvider>
      </InputProvider>
    </I18nextProvider>,
  )
}

describe('TrashScreen', () => {
  it('plans a per-video conflict and sends the chosen restore decision', async () => {
    const restoreTrashEntry = vi.fn().mockResolvedValue({ restored: [] })
    renderTrash({ restoreTrashEntry })
    fireEvent.click(await screen.findByRole('button', { name: 'Restaurar film.mkv, film.es.srt' }))
    await waitFor(() =>
      expect(screen.getByRole('dialog', { name: 'Plan de restauración' })).toBeInTheDocument(),
    )
    fireEvent.click(
      within(screen.getByRole('dialog', { name: 'Plan de restauración' })).getByRole('button', {
        name: 'Restaurar',
      }),
    )
    await waitFor(() => {
      expect(restoreTrashEntry).toHaveBeenCalledWith(4, {
        decisions: [{ file: 'film.mkv', decision: 'keep_both', version: '1080p' }],
      })
    })
  })

  it('asks before emptying the trash', async () => {
    const emptyTrash = vi.fn().mockResolvedValue(undefined)
    renderTrash({ emptyTrash })
    fireEvent.click(await screen.findByRole('button', { name: 'Vaciar papelera' }))
    expect(screen.getByRole('dialog', { name: '¿Vaciar la papelera?' })).toBeInTheDocument()
    fireEvent.click(
      within(screen.getByRole('dialog', { name: '¿Vaciar la papelera?' })).getByRole('button', {
        name: 'Vaciar papelera',
      }),
    )
    await waitFor(() => {
      expect(emptyTrash).toHaveBeenCalledTimes(1)
    })
  })

  it('names each destructive trash action with its entry', async () => {
    renderTrash()
    expect(
      await screen.findByRole('button', { name: 'Restaurar film.mkv, film.es.srt' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Borrar para siempre film.mkv, film.es.srt' }),
    ).toBeInTheDocument()
  })
})
