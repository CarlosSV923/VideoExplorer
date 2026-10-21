import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from '@/shared/i18n'
import { InputProvider, useDispatch } from '@/shared/input'
import { MultiSelectMenu } from './MultiSelectMenu'

function renderMenu() {
  const onChange = vi.fn()
  let dispatch: ReturnType<typeof useDispatch> | undefined
  function Harness() {
    const [value, setValue] = useState<string[]>([])
    dispatch = useDispatch()
    return (
      <MultiSelectMenu
        label="Géneros"
        options={[
          { value: 'Drama', label: 'Drama' },
          { value: 'Comedia', label: 'Comedia' },
        ]}
        value={value}
        onChange={(next) => {
          onChange(next)
          setValue(next)
        }}
        clearLabel="Quitar filtro"
        multipleLabel="Géneros · 2"
      />
    )
  }
  render(
    <I18nextProvider i18n={createI18n('es')}>
      <InputProvider>
        <Harness />
      </InputProvider>
    </I18nextProvider>,
  )
  return {
    onChange,
    back: () => dispatch?.('back'),
    confirm: () => dispatch?.('confirm'),
  }
}

describe('MultiSelectMenu', () => {
  it('opens with focus on the first real checkbox and closes from the button and restores focus', async () => {
    renderMenu()
    const button = screen.getByRole('button', { name: 'Géneros' })
    fireEvent.click(button)
    const first = screen.getByRole('checkbox', { name: 'Drama' })
    expect(document.activeElement).toBe(first)
    expect(button).toHaveAttribute('aria-haspopup', 'true')
    expect(button).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(button)
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    await waitFor(() => {
      expect(document.activeElement).toBe(button)
    })
  })

  it('closes with Escape and returns focus', async () => {
    renderMenu()
    const button = screen.getByRole('button', { name: 'Géneros' })
    fireEvent.click(button)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    await waitFor(() => {
      expect(document.activeElement).toBe(button)
    })
  })

  it('closes with the gamepad back action and returns focus', async () => {
    const { back } = renderMenu()
    const button = screen.getByRole('button', { name: 'Géneros' })
    fireEvent.click(button)
    act(() => {
      back()
    })
    await waitFor(() => {
      expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
      expect(document.activeElement).toBe(button)
    })
  })

  it('closes on outside pointer interaction', () => {
    renderMenu()
    fireEvent.click(screen.getByRole('button', { name: 'Géneros' }))
    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('moves focus with arrows, marks multiple options and clears the filter', () => {
    const { onChange } = renderMenu()
    fireEvent.click(screen.getByRole('button', { name: 'Géneros' }))
    const drama = screen.getByRole('checkbox', { name: 'Drama' })
    const comedy = screen.getByRole('checkbox', { name: 'Comedia' })
    fireEvent.keyDown(drama, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(comedy)
    fireEvent.click(drama)
    fireEvent.click(comedy)
    expect(onChange).toHaveBeenNthCalledWith(1, ['Drama'])
    expect(onChange).toHaveBeenNthCalledWith(2, ['Drama', 'Comedia'])
    fireEvent.click(screen.getByRole('button', { name: 'Quitar filtro' }))
    expect(onChange).toHaveBeenCalledWith([])
  })

  it('marks the focused checkbox with Space and the gamepad A action', async () => {
    const { onChange, confirm } = renderMenu()
    fireEvent.click(screen.getByRole('button', { name: 'Géneros' }))
    const drama = screen.getByRole('checkbox', { name: 'Drama' })
    await userEvent.keyboard(' ')
    expect(drama).toBeChecked()
    expect(onChange).toHaveBeenCalledWith(['Drama'])
    drama.focus()
    act(() => {
      confirm()
    })
    expect(drama).not.toBeChecked()
    expect(onChange).toHaveBeenLastCalledWith([])
  })

  it('uses the selected genre or plural count for the button text', () => {
    const { rerender } = render(
      <I18nextProvider i18n={createI18n('es')}>
        <InputProvider>
          <MultiSelectMenu
            label="Géneros"
            options={[{ value: 'Drama', label: 'Drama' }]}
            value={['Drama']}
            onChange={vi.fn()}
            clearLabel="Quitar filtro"
            multipleLabel="Géneros · 2"
          />
        </InputProvider>
      </I18nextProvider>,
    )
    expect(screen.getByRole('button', { name: 'Drama' })).toHaveClass('border-accent')
    rerender(
      <I18nextProvider i18n={createI18n('es')}>
        <InputProvider>
          <MultiSelectMenu
            label="Géneros"
            options={[{ value: 'Drama', label: 'Drama' }]}
            value={['Drama', 'Comedia']}
            onChange={vi.fn()}
            clearLabel="Quitar filtro"
            multipleLabel="Géneros · 2"
          />
        </InputProvider>
      </I18nextProvider>,
    )
    expect(screen.getByRole('button', { name: 'Géneros · 2' })).toBeInTheDocument()
  })
})
