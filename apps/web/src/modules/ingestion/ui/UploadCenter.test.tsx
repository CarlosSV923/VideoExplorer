import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'

import { IngestionContext, type IngestionPorts } from '../application/ports'
import type { UploadJob } from '../domain/types'
import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'
import { Header } from '@/app/LibraryLayout'
import { UploadUiProvider } from './UploadCenter'

vi.mock('@/app/config', () => ({ dataSource: 'demo' }))

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

function renderCenter(ports: Partial<IngestionPorts> = {}, demo = false) {
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
            <UploadUiProvider demo={demo}>
              <Header />
            </UploadUiProvider>
          </IngestionContext>
        </QueryClientProvider>
      </InputProvider>
    </I18nextProvider>,
  )
}

describe('UploadCenter', () => {
  it.each([
    ['escritorio', false, 'complementary'],
    ['móvil', true, 'dialog'],
  ] as const)('monta solo la variante de %s según el ancho', (_, mobile, role) => {
    const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: mobile,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
    try {
      renderCenter()
      fireEvent.click(screen.getByRole('button', { name: 'Subidas' }))
      expect(screen.getByRole(role, { name: 'Subidas' })).toBeInTheDocument()
      expect(
        role === 'dialog'
          ? screen.queryByRole('complementary', { name: 'Subidas' })
          : screen.queryByRole('dialog', { name: 'Subidas' }),
      ).not.toBeInTheDocument()
    } finally {
      if (originalMatchMedia) Object.defineProperty(window, 'matchMedia', originalMatchMedia)
      else Reflect.deleteProperty(window, 'matchMedia')
    }
  })

  it('keeps Subir separate from the uploads panel and offers it in the empty state', async () => {
    renderCenter()

    fireEvent.click(screen.getByRole('button', { name: 'Subir' }))
    expect(screen.queryByRole('complementary', { name: 'Subidas' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Subidas' }))
    expect(await screen.findByText('No hay subidas')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Subir' })).toHaveLength(2)
  })

  it('keeps the examples button out of the mobile header and at the top of the uploads sheet', async () => {
    const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: true,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
    try {
      renderCenter({}, true)
      const header = document.querySelector('header')
      if (!header) throw new Error('Falta la cabecera')
      const headerExamples = within(header).getByRole('button', {
        name: 'Probar con archivos de ejemplo',
      })
      expect(headerExamples).toHaveClass('hidden', 'md:inline-flex')
      fireEvent.click(screen.getByRole('button', { name: 'Subidas' }))
      const sheet = await screen.findByRole('dialog', { name: 'Subidas' })
      const examples = within(sheet).getAllByRole('button', {
        name: 'Probar con archivos de ejemplo',
      })
      const emptyTitle = await within(sheet).findByText('No hay subidas')
      const firstExample = examples.at(0)
      if (!firstExample) throw new Error('Falta el botón de ejemplos en la hoja')
      expect(
        firstExample.compareDocumentPosition(emptyTitle) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
    } finally {
      if (originalMatchMedia) Object.defineProperty(window, 'matchMedia', originalMatchMedia)
      else Reflect.deleteProperty(window, 'matchMedia')
    }
  })

  it('announces the active count on Subidas', async () => {
    renderCenter({
      jobs: vi
        .fn()
        .mockResolvedValue([job('uploading'), { ...job('needs_password'), id: 'job-2' }]),
    })

    expect(await screen.findByRole('button', { name: 'Subidas, 2 activas' })).toBeInTheDocument()
    expect(screen.getByText('Subidas · 2 activas')).toBeInTheDocument()
  })

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
    expect(await screen.findByText('Sin relación')).toBeInTheDocument()
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

  it('shows one set-level password field for a multipart archive', async () => {
    const first = {
      ...job('needs_password'),
      fileName: 'Inception.2010.part1.rar',
      batch: { ...job('needs_password').batch, id: 'batch-multipart' },
    }
    const second = {
      ...job('needs_password'),
      id: 'job-2',
      fileName: 'Inception.2010.part2.rar',
      batch: first.batch,
    }
    renderCenter({ jobs: vi.fn().mockResolvedValue([first, second]) })
    fireEvent.click(await screen.findByRole('button', { name: /Subidas/ }))
    expect(
      await screen.findByText('Esta contraseña se aplica a todos los volúmenes del conjunto.'),
    ).toBeInTheDocument()
    expect(screen.getAllByLabelText('Contraseña')).toHaveLength(1)
  })
})
