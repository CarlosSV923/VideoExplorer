import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'

import { useAction } from '../input'
import { cx } from './cx'

export interface TabOption {
  value: string
  label: ReactNode
  disabled?: boolean
}

export function Tabs({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: readonly TabOption[]
  value: string
  onChange: (value: string) => void
}) {
  const list = useRef<HTMLDivElement>(null)
  const move = (delta: number) => {
    const enabled = options.filter((option) => !option.disabled)
    const index = Math.max(
      0,
      enabled.findIndex((option) => option.value === value),
    )
    const next = enabled[(index + delta + enabled.length) % enabled.length]
    if (next) onChange(next.value)
  }
  useAction('prev', () => {
    move(-1)
    return true
  })
  useAction('next', () => {
    move(1)
    return true
  })
  return (
    <div
      ref={list}
      role="tablist"
      aria-label={label}
      className="flex max-w-full gap-2 overflow-x-auto"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={option.value === value}
          tabIndex={option.value === value ? 0 : -1}
          disabled={option.disabled}
          onClick={() => {
            onChange(option.value)
          }}
          className={cx(
            'min-h-control-sm shrink-0 rounded-pill border border-control px-4 text-body-sm font-bold',
            option.value === value
              ? 'border-ink-1 bg-ink-1 text-on-accent'
              : 'text-ink-2 hover:bg-hover',
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

export function SectionTabs({
  label,
  tabs,
  current,
}: {
  label: string
  tabs: readonly { href: string; label: ReactNode; value: string }[]
  current: string
}) {
  return (
    <nav aria-label={label} className="flex flex-wrap gap-2">
      {tabs.map((tab) => (
        <a
          key={tab.value}
          href={tab.href}
          aria-current={tab.value === current ? 'page' : undefined}
          className={cx(
            'min-h-control-sm rounded-pill px-4 py-2 text-body-sm font-bold no-underline',
            tab.value === current ? 'bg-ink-1 text-on-accent' : 'border border-control text-ink-2',
          )}
        >
          {tab.label}
        </a>
      ))}
    </nav>
  )
}

export interface MenuItem {
  label: string
  onSelect: () => void
  tone?: StatusToneForMenu
}
type StatusToneForMenu = 'default' | 'danger'

export function Menu({
  label,
  items,
  children,
}: {
  label: string
  items: readonly MenuItem[]
  children: ReactNode
}) {
  const id = useId()
  const button = useRef<HTMLButtonElement>(null)
  const box = useRef<HTMLDivElement>(null)
  const menu = useRef<HTMLUListElement>(null)
  const [open, setOpen] = useState(false)
  const [menuPosition, setMenuPosition] = useState<{ left: number; top: number } | null>(null)
  useAction(
    'back',
    () => {
      if (!open) return false
      setOpen(false)
      button.current?.focus()
      return true
    },
    open,
  )
  useEffect(() => {
    if (!open) return
    menu.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
    const closeOnOutside = (event: PointerEvent) => {
      if (!box.current?.contains(event.target as Node)) {
        setOpen(false)
      }
    }
    window.addEventListener('pointerdown', closeOnOutside)
    return () => {
      window.removeEventListener('pointerdown', closeOnOutside)
    }
  }, [open])
  useLayoutEffect(() => {
    if (!open || !button.current || !menu.current) {
      setMenuPosition(null)
      return
    }
    const trigger = button.current.getBoundingClientRect()
    const width = menu.current.getBoundingClientRect().width
    const margin = 8
    setMenuPosition({
      left: Math.max(margin, Math.min(trigger.right - width, window.innerWidth - width - margin)),
      top: trigger.bottom + 8,
    })
  }, [open])
  const onKeyDown = (event: KeyboardEvent) => {
    if (!open) return
    const entries = Array.from(
      box.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [],
    )
    const index = entries.indexOf(document.activeElement as HTMLElement)
    const next =
      event.key === 'ArrowDown'
        ? entries[(index + 1) % entries.length]
        : event.key === 'ArrowUp'
          ? entries[(index - 1 + entries.length) % entries.length]
          : undefined
    if (event.key === 'Escape') {
      event.preventDefault()
      setOpen(false)
      button.current?.focus()
    } else if (next) {
      event.preventDefault()
      next.focus()
    }
  }
  return (
    <div ref={box} className="relative inline-flex" onKeyDown={onKeyDown}>
      <button
        ref={button}
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        onClick={() => {
          setOpen((current) => !current)
        }}
        className="inline-flex min-h-control-sm items-center justify-center rounded-pill border border-control px-4 text-body-sm font-bold"
      >
        {children}
      </button>
      {open && (
        <ul
          ref={menu}
          id={id}
          role="menu"
          aria-label={label}
          style={
            menuPosition
              ? { position: 'fixed', left: menuPosition.left, top: menuPosition.top }
              : undefined
          }
          className="z-30 min-w-60 rounded-lg border border-control bg-surface-2 p-1.5 shadow-xl"
        >
          {items.map((item) => (
            <li key={item.label} role="none">
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setOpen(false)
                  item.onSelect()
                }}
                className={cx(
                  'flex min-h-control-sm w-full items-center rounded-md border-0 bg-transparent px-3 text-left text-body-sm font-semibold',
                  item.tone === 'danger' ? 'text-danger' : 'text-ink-1',
                )}
              >
                {item.label}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
