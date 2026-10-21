import {
  useId,
  useRef,
  type AnchorHTMLAttributes,
  type ButtonHTMLAttributes,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from './cx'
import { AppLink } from './AppLink'
import { Spinner } from './Feedback'
import { buttonClass, type ButtonSize, type ButtonVariant } from './buttonClass'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: ReactNode
  loading?: boolean
  disabledReason?: string
}

/** Disabled controls stay focusable so the visible reason is discoverable. */
export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  loading = false,
  disabled = false,
  disabledReason,
  className,
  children,
  type = 'button',
  onClick,
  ...rest
}: ButtonProps) {
  const reasonId = useId()
  const { t } = useTranslation()
  const unavailable = disabled || loading
  const button = (
    <button
      type={type}
      aria-disabled={unavailable || undefined}
      aria-busy={loading || undefined}
      aria-describedby={unavailable && disabledReason ? reasonId : undefined}
      className={cx(
        buttonClass(variant, size),
        unavailable && 'cursor-not-allowed bg-surface-3 text-ink-2',
        className,
      )}
      onClick={(event) => {
        if (unavailable) {
          event.preventDefault()
          return
        }
        onClick?.(event)
      }}
      {...rest}
    >
      {loading ? <Spinner label={t('common.loading')} /> : icon}
      {children}
    </button>
  )
  if (!unavailable || !disabledReason) return button
  return (
    <span className="inline-flex flex-col items-start gap-1">
      {button}
      <span id={reasonId} className="text-caption text-ink-2">
        {disabledReason}
      </span>
    </span>
  )
}

interface IconButtonProps extends Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  'aria-label' | 'children'
> {
  label: string
  tone?: 'default' | 'danger'
  variant?: 'outline' | 'ghost' | 'overlay'
  children: ReactNode
}

export function IconButton({
  label,
  tone = 'default',
  variant = 'ghost',
  className,
  children,
  type = 'button',
  ...rest
}: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      className={cx(
        'inline-flex size-control-sm shrink-0 items-center justify-center rounded-full transition duration-120 ease-out hover:bg-white/8',
        variant === 'outline' && 'border border-control',
        variant === 'overlay' && 'bg-black/35 backdrop-blur-sm',
        tone === 'danger' ? 'text-danger' : 'text-ink-1',
        className,
      )}
      {...rest}
    >
      {children}
    </button>
  )
}

interface LinkButtonProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  href: string
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: ReactNode
}

function isAppRouteHref(href: string): boolean {
  return (
    href === '/' ||
    /^\/(?:movies|series|collections|search|unassigned|settings|login)(?:\/|\?|$)/.test(href)
  )
}

export function LinkButton({
  variant = 'secondary',
  size = 'md',
  icon,
  className,
  children,
  ...rest
}: LinkButtonProps) {
  const linkClass = cx(buttonClass(variant, size), className)
  return isAppRouteHref(rest.href) ? (
    <AppLink className={linkClass} {...rest}>
      {icon}
      {children}
    </AppLink>
  ) : (
    <a className={linkClass} {...rest}>
      {icon}
      {children}
    </a>
  )
}

interface FileButtonProps {
  onFiles: (files: File[]) => void
  multiple?: boolean
  accept?: string
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: ReactNode
  className?: string
  shortcut?: boolean
  children: ReactNode
}

export function FileButton({
  onFiles,
  multiple = true,
  accept,
  variant = 'primary',
  size = 'md',
  icon,
  className,
  shortcut = false,
  children,
}: FileButtonProps) {
  const input = useRef<HTMLInputElement>(null)
  return (
    <label className={cx(buttonClass(variant, size), 'relative cursor-pointer', className)}>
      {icon}
      {children}
      <input
        ref={input}
        type="file"
        multiple={multiple}
        accept={accept}
        data-shortcut={shortcut ? 'upload' : undefined}
        className="absolute inset-0 size-full cursor-pointer opacity-0"
        onChange={(event) => {
          const files = Array.from(event.currentTarget.files ?? [])
          if (files.length) onFiles(files)
          if (input.current) input.current.value = ''
        }}
      />
    </label>
  )
}

export type { ButtonSize, ButtonVariant } from './buttonClass'
