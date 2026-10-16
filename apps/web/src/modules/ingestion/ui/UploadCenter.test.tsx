import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'

import { IngestionContext, type IngestionPorts } from '../application/ports'
import type { UploadJob } from '../domain/types'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { UploadUiProvider } from './UploadCenter'

const job = (status: UploadJob['status']): UploadJob => ({
  id: 'job-1',
  batch: { id: 'batch-1', name: 'batch', relation: 'none', createdAt: '2026-01-01T00:00:00Z' },
  fileName: 'secret.7z',
  relativePath: '',
  size: 10,
  received: 10,
  progress: 0,
  status,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

function renderCenter(ports: Partial<IngestionPorts> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const full = {
    createBatch: vi.fn().mockResolvedValue({
      id: 'batch-1',
      name: 'batch',
      relation: 'none',
      createdAt: '2026-01-01T00:00:00Z',
    }),
    upload: vi.fn().mockReturnValue({ abort: vi.fn() }),
    jobs: vi.fn().mockResolvedValue([]),
    watchJobs: vi.fn(() => () => undefined),
    submitPassword: vi.fn().mockResolvedValue(job('extracting')),
    retryJob: vi.fn().mockResolvedValue(job('extracting')),
    dismissJob: vi.fn().mockResolvedValue(job('discarded')),
    cancelUpload: vi.fn().mockResolvedValue(undefined),
    ...ports,
  } as IngestionPorts
  return render(
    <I18nextProvider i18n={createI18n('es')}>
      <InputProvider>
        <QueryClientProvider client={client}>
          <IngestionContext value={full}>
            <UploadUiProvider>
              <button type="button">Abrir selector</button>
            </UploadUiProvider>
          </IngestionContext>
        </QueryClientProvider>
      </InputProvider>
    </I18nextProvider>,
  )
}

describe('UploadCenter', () => {
  it('asks once for the relation and sends partsTotal for each multipart volume', async () => {
    const upload = vi
      .fn<NonNullable<IngestionPorts['upload']>>()
      .mockReturnValue({ abort: vi.fn() })
    const ports = { upload }
    const { container } = renderCenter(ports)
    const input = container.querySelector('input[type="file"]')
    const files = [new File(['a'], 'film.part1.rar'), new File(['b'], 'film.part2.rar')]
    fireEvent.change(input as HTMLInputElement, { target: { files } })
    expect(
      screen.getByRole('dialog', { name: '¿Cómo se relacionan estos archivos?' }),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Partes de una misma película o serie' }))
    await waitFor(() => {
      expect(ports.upload).toHaveBeenCalledTimes(2)
    })
    expect(ports.upload.mock.calls.map((call) => call[1])).toEqual([
      expect.objectContaining({ partsTotal: 2 }),
      expect.objectContaining({ partsTotal: 2 }),
    ])
  })

  it('shows a password field for needs_password and sends it through the port', async () => {
    const submitPassword = vi.fn().mockResolvedValue(job('extracting'))
    const ports = { jobs: vi.fn().mockResolvedValue([job('needs_password')]), submitPassword }
    const { container } = renderCenter(ports)
    const input = container.querySelector('input[type="file"]')
    fireEvent.change(input as HTMLInputElement, {
      target: { files: [new File(['a'], 'secret.7z')] },
    })
    await waitFor(() => {
      expect(screen.getAllByLabelText('Contraseña').length).toBeGreaterThan(0)
    })
    const password = screen.getAllByLabelText('Contraseña')[0]
    const continueButton = screen.getAllByRole('button', { name: 'Continuar' })[0]
    if (!password || !continueButton)
      throw new Error('El formulario de contraseña no está completo')
    fireEvent.change(password, { target: { value: 'cc' } })
    fireEvent.click(continueButton)
    await waitFor(() => {
      expect(submitPassword).toHaveBeenCalledWith('job-1', 'cc')
    })
  })
})
