import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useGamepadConnected } from '../input'
import { cx } from './cx'

export type GlyphName = 'A' | 'B' | 'X' | 'Y' | 'LB' | 'RB' | 'RT' | 'DPAD'
export interface HelpAction {
  glyph: GlyphName
  label: string
}

export function Glyph({ name }: { name: GlyphName }) {
  const { t } = useTranslation()
  const label = t(`glyph.${name}`)
  if (name === 'DPAD')
    return (
      <svg role="img" aria-label={label} width="22" height="22" viewBox="0 0 22 22">
        <path d="M8 1h6v7h7v6h-7v7H8v-7H1V8h7z" fill="currentColor" />
      </svg>
    )
  const wide = name === 'LB' || name === 'RB' || name === 'RT'
  return (
    <span
      role="img"
      aria-label={label}
      className={cx(
        'inline-flex h-6 items-center justify-center bg-ink-1 font-bold text-on-accent',
        wide ? 'rounded-t-sm rounded-b-xl px-2 text-caption' : 'w-6 rounded-full text-caption',
      )}
    >
      <span aria-hidden="true">{name}</span>
    </span>
  )
}

export function HelpBar({
  actions,
  children,
  aside,
}: {
  actions: readonly HelpAction[]
  children?: ReactNode
  aside?: ReactNode
}) {
  const gamepad = useGamepadConnected()
  if (!gamepad && !children && !aside) return null
  return (
    <footer className="flex flex-wrap items-center justify-center gap-x-8 gap-y-3 bg-bg-sunken px-4 py-3 text-body-sm sm:px-6">
      {gamepad
        ? actions.map((action) => (
            <span
              key={`${action.glyph}-${action.label}`}
              className="inline-flex items-center gap-2"
            >
              <Glyph name={action.glyph} />
              {action.label}
            </span>
          ))
        : children}
      {aside}
    </footer>
  )
}

export function ShortcutBar({
  groups,
}: {
  groups: readonly { title: string; keys: readonly { key: string; label: string }[] }[]
}) {
  return (
    <footer className="flex flex-wrap items-center justify-center gap-x-8 gap-y-3 bg-bg-sunken px-4 py-3 text-body-sm">
      {groups.map((group) => (
        <span key={group.title} className="inline-flex flex-wrap items-center gap-2">
          <span className="text-ink-2">{group.title}</span>
          {group.keys.map((item) => (
            <span key={`${item.key}-${item.label}`} className="inline-flex items-center gap-1.5">
              <kbd className="rounded-sm border border-control px-1.5 font-mono text-caption text-ink-1">
                {item.key}
              </kbd>
              <span>{item.label}</span>
            </span>
          ))}
        </span>
      ))}
    </footer>
  )
}

export function PageHeader({
  back,
  title,
  subtitle,
  actions,
}: {
  back?: ReactNode
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="page-padding flex flex-wrap items-end justify-between gap-4 border-b border-line pt-7 pb-5">
      <div className="flex min-w-0 flex-col gap-2">
        {back}
        <h1 className="m-0 text-page-title font-display break-words uppercase">{title}</h1>
        {subtitle}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-3">{actions}</div>}
    </header>
  )
}
