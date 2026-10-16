import type { ReactNode } from 'react'

import { cx } from './cx'

export type StatusTone = 'neutral' | 'success' | 'warning' | 'info' | 'danger'

const toneClasses: Record<StatusTone, string> = {
  neutral: 'bg-surface-3 text-ink-1',
  success: 'border border-accent bg-success-surface text-success-ink',
  warning: 'border border-warning bg-warning-surface text-warning-ink',
  info: 'border border-info bg-info-surface text-info-ink',
  danger: 'border border-danger bg-danger-surface text-danger-ink',
}

export function Chip({
  children,
  tone = 'neutral',
  solid = false,
}: {
  children: ReactNode
  tone?: StatusTone
  solid?: boolean
}) {
  return (
    <span
      className={cx(
        'inline-flex min-h-8 items-center rounded-pill px-3 text-caption font-bold tracking-label uppercase',
        toneClasses[tone],
        solid && 'shadow-sm',
      )}
    >
      {children}
    </span>
  )
}

export function StatusChip(props: { children: ReactNode; tone?: StatusTone; solid?: boolean }) {
  return <Chip {...props} />
}

interface ChoiceOption<T extends string> {
  value: T
  label: ReactNode
  disabled?: boolean
}

export function SegmentedControl<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: readonly ChoiceOption<T>[]
  value: T
  onChange: (value: T) => void
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="inline-flex flex-wrap gap-1 rounded-pill bg-surface-2 p-1"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={option.value === value}
          disabled={option.disabled}
          onClick={() => {
            onChange(option.value)
          }}
          className={cx(
            'min-h-control-sm rounded-pill border-0 px-4 text-body-sm font-bold transition',
            option.value === value ? 'bg-ink-1 text-on-accent' : 'text-ink-2 hover:bg-hover',
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

export function ChipGroup<T extends string>({
  legend,
  name,
  options,
  value,
  onChange,
}: {
  legend: string
  name: string
  options: readonly ChoiceOption<T>[]
  value?: T
  onChange: (value: T) => void
}) {
  return (
    <fieldset className="m-0 flex min-w-0 flex-wrap items-center gap-2 border-0 p-0">
      <legend className="sr-only">{legend}</legend>
      {options.map((option) => (
        <label
          key={option.value}
          className={cx(
            'relative inline-flex min-h-control-sm cursor-pointer items-center rounded-pill px-4 text-body-sm font-bold has-[:focus-visible]:outline-3 has-[:focus-visible]:outline-offset-3 has-[:focus-visible]:outline-accent',
            value === option.value
              ? 'bg-accent text-on-accent'
              : 'border border-control text-ink-1 hover:bg-hover',
          )}
        >
          <input
            type="radio"
            name={name}
            value={option.value}
            checked={value === option.value}
            disabled={option.disabled}
            onChange={() => {
              onChange(option.value)
            }}
            className="absolute size-px opacity-0"
          />
          {option.label}
        </label>
      ))}
    </fieldset>
  )
}
