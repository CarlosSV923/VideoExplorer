import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'

import { createI18n } from '../i18n'
import { InputProvider } from '../input'
import {
  Button,
  ConfirmDialog,
  Dialog,
  HelpBar,
  Menu,
  Poster,
  ProgressBar,
  SearchField,
  TextField,
} from '.'

function wrap(ui: ReactNode) {
  return render(
    <I18nextProvider i18n={createI18n('es')}>
      <InputProvider>{ui}</InputProvider>
    </I18nextProvider>,
  )
}

describe('UI primitives', () => {
  it('keeps a disabled reason accessible while remaining focusable', () => {
    wrap(
      <Button disabled disabledReason="Decide un duplicado">
        Guardar
      </Button>,
    )
    const button = screen.getByRole('button', { name: 'Guardar' })
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).not.toBeDisabled()
    expect(button).toHaveAccessibleDescription('Decide un duplicado')
  })

  it('associates a field error with aria-invalid', () => {
    wrap(<TextField label="Nombre" error="El nombre es obligatorio" />)
    const field = screen.getByRole('textbox', { name: 'Nombre' })
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAccessibleDescription('El nombre es obligatorio')
  })

  it('focuses search with the slash shortcut', async () => {
    const user = userEvent.setup()
    wrap(<SearchField label="Buscar" shortcut />)
    await user.keyboard('/')
    expect(screen.getByRole('searchbox', { name: 'Buscar' })).toHaveFocus()
  })

  it('includes poster metadata and chips in its accessible name', () => {
    wrap(<Poster href="#dark" title="Dark" meta="2017" accessibleChips={['Serie', 'Faltan 4']} />)
    expect(screen.getByRole('link', { name: 'Dark, 2017, Serie, Faltan 4' })).toBeInTheDocument()
  })

  it('exposes progress and dialog focus semantics', async () => {
    const user = userEvent.setup()
    function Example() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button
            type="button"
            onClick={() => {
              setOpen(true)
            }}
          >
            Abrir
          </button>
          <ProgressBar value={63} label="Extracción" />
          {open && (
            <Dialog
              title="Confirmar"
              onClose={() => {
                setOpen(false)
              }}
              actions={
                <Button
                  onClick={() => {
                    setOpen(false)
                  }}
                >
                  Aceptar
                </Button>
              }
            >
              <p>Contenido</p>
            </Dialog>
          )}
        </>
      )
    }
    const { container } = wrap(<Example />)
    expect(screen.getByRole('progressbar', { name: 'Extracción' })).toHaveAttribute(
      'aria-valuenow',
      '63',
    )
    const opener = screen.getByRole('button', { name: 'Abrir' })
    await user.click(opener)
    expect(screen.getByRole('dialog', { name: 'Confirmar' })).toHaveAttribute('aria-modal', 'true')
    expect(screen.getByRole('button', { name: 'Cerrar' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Aceptar' })).toHaveFocus()
    expect(container).toHaveAttribute('inert')
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(opener).toHaveFocus()
  })

  it('focuses cancel before a destructive confirmation action', () => {
    wrap(
      <ConfirmDialog
        title="Borrar archivo"
        confirmLabel="Borrar"
        tone="danger"
        onConfirm={() => undefined}
        onCancel={() => undefined}
      >
        Esta acción no se puede deshacer.
      </ConfirmDialog>,
    )

    expect(screen.getByRole('button', { name: 'Cancelar' })).toHaveFocus()
  })

  it('only renders gamepad glyphs when a control is connected', () => {
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [] })
    wrap(
      <HelpBar actions={[{ glyph: 'A', label: 'Aceptar' }]}>
        <button type="button">Ayuda</button>
      </HelpBar>,
    )
    expect(screen.getByRole('button', { name: 'Ayuda' })).toBeInTheDocument()
    act(() => {
      Object.defineProperty(navigator, 'getGamepads', {
        configurable: true,
        value: () => [{ buttons: [], axes: [] }],
      })
      window.dispatchEvent(new Event('gamepadconnected'))
    })
    expect(screen.getByRole('img', { name: 'Botón A' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Ayuda' })).not.toBeInTheDocument()
    Reflect.deleteProperty(navigator, 'getGamepads')
  })

  it('keeps an open menu inside a narrow viewport', () => {
    const width = Object.getOwnPropertyDescriptor(window, 'innerWidth')
    const rects = vi
      .spyOn(HTMLElement.prototype, 'getBoundingClientRect')
      .mockImplementation(function (this: HTMLElement) {
        if (this instanceof HTMLButtonElement) return new DOMRect(0, 100, 40, 40)
        if (this instanceof HTMLUListElement) return new DOMRect(0, 0, 240, 120)
        return new DOMRect()
      })
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 320 })
    try {
      wrap(
        <Menu label="Acciones" items={[{ label: 'Descargar', onSelect: () => undefined }]}>
          Más
        </Menu>,
      )
      fireEvent.click(screen.getByRole('button', { name: 'Acciones' }))
      expect(screen.getByRole('menu')).toHaveStyle({ position: 'fixed', left: '8px' })
    } finally {
      rects.mockRestore()
      if (width) Object.defineProperty(window, 'innerWidth', width)
    }
  })
})
