import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef, useState } from 'react'

import { InputProvider, useLetterShortcuts } from '.'

function Example() {
  const root = useRef<HTMLDivElement>(null)
  const [selected, setSelected] = useState('none')
  useLetterShortcuts(root, {
    r: () => {
      setSelected('replace')
    },
    a: () => {
      setSelected('accept')
    },
  })
  return (
    <div ref={root}>
      <button
        type="button"
        onClick={() => {
          setSelected('focus')
        }}
      >
        Video
      </button>
      <input aria-label="Nombre" />
      <output>{selected}</output>
    </div>
  )
}

describe('useLetterShortcuts', () => {
  it('runs only inside the container and outside text fields', async () => {
    const user = userEvent.setup()
    render(
      <InputProvider>
        <Example />
      </InputProvider>,
    )
    const card = screen.getByRole('button', { name: 'Video' })
    const field = screen.getByRole('textbox', { name: 'Nombre' })
    await user.click(card)
    await user.keyboard('r')
    expect(screen.getByText('replace')).toBeInTheDocument()
    await user.click(field)
    await user.keyboard('a')
    expect(screen.getByText('replace')).toBeInTheDocument()
    await user.click(document.body)
    await user.keyboard('r')
    expect(screen.getByText('replace')).toBeInTheDocument()
  })
})
