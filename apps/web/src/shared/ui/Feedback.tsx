import { useState, type ReactNode } from 'react'

import { cx } from './cx'
import { ChevronDownIcon, ErrorIcon, InfoIcon, WarningIcon } from './icons'

export function Spinner({ label, size = 18 }: { label: string; size?: number }) {
  return (
    <span role="status" aria-label={label} className="inline-flex">
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        aria-hidden="true"
        className="animate-spin"
      >
        <circle
          cx="12"
          cy="12"
          r="9"
          fill="none"
          stroke="currentColor"
          strokeOpacity=".3"
          strokeWidth="3"
        />
        <path
          d="M21 12a9 9 0 0 0-9-9"
          fill="none"
          stroke="currentColor"
          strokeWidth="3"
          strokeLinecap="round"
        />
      </svg>
    </span>
  )
}

export type BannerTone = 'warning' | 'danger' | 'info'

const bannerTones: Record<BannerTone, string> = {
  warning: 'border-warning bg-warning-surface text-warning-ink',
  danger: 'border-danger bg-danger-surface text-danger-ink',
  info: 'border-info bg-info-surface text-info-ink',
}

export function Banner({
  tone = 'warning',
  live = tone === 'danger' ? 'assertive' : 'polite',
  action,
  children,
}: {
  tone?: BannerTone
  live?: 'polite' | 'assertive'
  action?: ReactNode
  children: ReactNode
}) {
  const Icon = tone === 'danger' ? ErrorIcon : tone === 'info' ? InfoIcon : WarningIcon
  return (
    <div
      role={live === 'assertive' ? 'alert' : 'status'}
      className={cx(
        'flex flex-wrap items-start justify-between gap-3 rounded-md border px-4 py-3 text-body-sm',
        bannerTones[tone],
      )}
    >
      <span className="flex min-w-0 items-start gap-3">
        <Icon className="mt-0.5 shrink-0" />
        <span>{children}</span>
      </span>
      {action}
    </div>
  )
}

export interface HealthCheck {
  name: string
  status: 'ok' | 'warning' | 'error'
  detail?: string
}

export function HealthBanner({
  checks,
  title,
  copyLabel,
}: {
  checks: readonly HealthCheck[]
  title: string
  copyLabel: string
}) {
  const [open, setOpen] = useState(false)
  const broken = checks.filter((check) => check.status !== 'ok')
  if (!broken.length) return null
  return (
    <Banner
      tone="danger"
      action={
        <button
          type="button"
          className="inline-flex min-h-control-sm items-center gap-2 rounded-pill border border-danger px-3 font-bold"
          onClick={() => {
            setOpen((value) => !value)
          }}
          aria-expanded={open}
        >
          {title}
          <ChevronDownIcon size={16} />
        </button>
      }
    >
      <div className="min-w-0">
        <strong>{title}</strong>
        {open && (
          <div className="mt-2 flex flex-col gap-2">
            <ul className="m-0 list-none p-0">
              {broken.map((check) => (
                <li key={check.name}>
                  {check.name}
                  {check.detail ? `: ${check.detail}` : ''}
                </li>
              ))}
            </ul>
            <button
              type="button"
              className="text-left font-bold underline"
              onClick={() => {
                const clipboard = (navigator as unknown as { clipboard?: Clipboard }).clipboard
                if (clipboard)
                  void clipboard.writeText(
                    broken.map((check) => `${check.name}: ${check.detail ?? ''}`).join('\n'),
                  )
              }}
            >
              {copyLabel}
            </button>
          </div>
        )}
      </div>
    </Banner>
  )
}

export function ProgressBar({
  value,
  label,
  tone = 'upload',
}: {
  value: number
  label: string
  tone?: 'upload' | 'extract'
}) {
  const percentage = Math.min(100, Math.max(0, Math.round(value)))
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuenow={percentage}
      aria-valuemin={0}
      aria-valuemax={100}
      className="h-2 overflow-hidden rounded-pill bg-line"
    >
      <div
        className={cx(
          'h-full transition-[width] duration-200 ease-linear',
          tone === 'extract' ? 'bg-progress-extract' : 'bg-progress-upload',
        )}
        style={{ width: `${String(percentage)}%` }}
      />
    </div>
  )
}

export function EmptyState({
  icon,
  title,
  body,
  action,
}: {
  icon?: ReactNode
  title: string
  body: string
  action?: ReactNode
}) {
  return (
    <section className="flex min-h-80 flex-col items-center justify-center gap-3 rounded-xl border border-line p-8 text-center">
      <span aria-hidden="true" className="text-ink-2">
        {icon}
      </span>
      <h2 className="m-0 text-heading font-bold">{title}</h2>
      <p className="m-0 max-w-[36ch] text-body text-ink-2">{body}</p>
      {action}
    </section>
  )
}
