import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { useAction } from '@/shared/input'
import { cx } from './cx'

export function MultiSelectMenu({
  label,
  options,
  value,
  onChange,
  clearLabel,
  multipleLabel,
}: {
  label: string
  options: readonly { value: string; label: string }[]
  value: readonly string[]
  onChange: (value: string[]) => void
  clearLabel: string
  multipleLabel: string
}) {
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const selectedLabel =
    value.length === 1 ? options.find((option) => option.value === value[0])?.label : null
  const close = () => {
    setOpen(false)
    requestAnimationFrame(() => trigger.current?.focus())
  }
  useAction(
    'back',
    () => {
      if (!open) return undefined
      close()
      return true
    },
    open,
  )
  useAction(
    'navigate',
    (direction) => {
      if (!open || (direction !== 'up' && direction !== 'down')) return undefined
      const controls = Array.from(
        root.current?.querySelectorAll<HTMLElement>(
          'input[type="checkbox"], button[data-menu-option]',
        ) ?? [],
      )
      const index = controls.indexOf(document.activeElement as HTMLElement)
      if (index < 0) return undefined
      controls[
        (index + (direction === 'down' ? 1 : controls.length - 1)) % controls.length
      ]?.focus()
      return true
    },
    open,
  )

  useEffect(() => {
    if (!open) return
    const top = (trigger.current?.getBoundingClientRect().bottom ?? 0) + 8
    panel.current?.style.setProperty('--menu-top', `${String(top)}px`)
    panel.current?.style.setProperty(
      '--menu-max-height',
      `${String(Math.min(window.innerHeight * 0.6, Math.max(0, window.innerHeight - top - 80)))}px`,
    )
    const first = root.current?.querySelector<HTMLInputElement>('input[type="checkbox"]')
    first?.focus()
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) close()
    }
    document.addEventListener('pointerdown', outside)
    const escape = (event: KeyboardEvent | globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        close()
      }
    }
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('keydown', escape)
    }
  }, [open])

  const moveFocus = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    const controls = Array.from(
      root.current?.querySelectorAll<HTMLElement>(
        'input[type="checkbox"], button[data-menu-option]',
      ) ?? [],
    )
    const index = controls.indexOf(document.activeElement as HTMLElement)
    if (index < 0) return
    event.preventDefault()
    controls[
      (index + (event.key === 'ArrowDown' ? 1 : controls.length - 1)) % controls.length
    ]?.focus()
  }

  return (
    <div ref={root} className="relative">
      <button
        ref={trigger}
        type="button"
        aria-haspopup="true"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => {
          if (open) close()
          else setOpen(true)
        }}
        className={cx(
          'inline-flex min-h-control-sm items-center gap-2 rounded-pill border bg-transparent px-4 text-body-sm font-bold text-ink-1 hover:bg-hover',
          value.length ? 'border-accent' : 'border-control',
        )}
      >
        {selectedLabel ?? (value.length > 1 ? multipleLabel : label)}
        <span
          aria-hidden="true"
          className={cx(
            'size-2 rotate-45 border-b-2 border-r-2 border-current transition-transform',
            open && '-rotate-[135deg]',
          )}
        />
      </button>
      {open && (
        <div
          ref={panel}
          id={id}
          onKeyDown={moveFocus}
          className="absolute left-0 top-full z-30 mt-2 max-h-[60vh] min-w-60 max-w-80 overflow-y-auto rounded-md border border-line bg-surface-2 p-2 shadow-xl max-sm:fixed max-sm:left-[var(--page-x,16px)] max-sm:right-[var(--page-x,16px)] max-sm:top-[var(--menu-top)] max-sm:mt-0 max-sm:max-h-[var(--menu-max-height)] max-sm:w-auto max-sm:min-w-0 max-sm:max-w-none"
        >
          {options.map((option) => (
            <label
              key={option.value}
              className="flex min-h-11 cursor-pointer items-center gap-3 rounded px-3 text-body-sm hover:bg-surface-3 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-accent"
            >
              <input
                type="checkbox"
                checked={value.includes(option.value)}
                onChange={(event) => {
                  onChange(
                    event.currentTarget.checked
                      ? [...value, option.value]
                      : value.filter((item) => item !== option.value),
                  )
                }}
                className="size-5 accent-accent"
              />
              {option.label}
            </label>
          ))}
          <div className="mt-2 border-t border-line pt-2">
            <button
              type="button"
              data-menu-option
              disabled={!value.length}
              onClick={() => {
                onChange([])
              }}
              className="min-h-11 w-full rounded px-3 text-left text-body-sm text-accent hover:bg-surface-3 disabled:cursor-not-allowed disabled:text-ink-2"
            >
              {clearLabel}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
