import {
  useId,
  useState,
  type ComponentPropsWithRef,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from './cx'
import { ChevronDownIcon, CloseIcon, ErrorIcon, EyeIcon, SearchIcon } from './icons'

type CommonFieldProps = { label: string; hideLabel?: boolean; hint?: ReactNode; error?: string }

function describedIds(id: string, hint: ReactNode, error?: string) {
  return [hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(' ') || undefined
}

export function TextField({
  label,
  hideLabel = false,
  hint,
  error,
  className,
  ...rest
}: CommonFieldProps & Omit<ComponentPropsWithRef<'input'>, 'size'>) {
  const id = useId()
  return (
    <FieldFrame id={id} label={label} hideLabel={hideLabel} hint={hint} error={error}>
      <input
        id={id}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedIds(id, hint, error)}
        className={cx(fieldClass, error ? 'border-2 border-danger' : 'border-control', className)}
        {...rest}
      />
    </FieldFrame>
  )
}

export function NumberField(
  props: CommonFieldProps & Omit<ComponentPropsWithRef<'input'>, 'size' | 'type'>,
) {
  return <TextField type="number" {...props} />
}

export function Select({
  label,
  hideLabel = false,
  hint,
  error,
  children,
  className,
  ...rest
}: CommonFieldProps & SelectHTMLAttributes<HTMLSelectElement> & { children: ReactNode }) {
  const id = useId()
  return (
    <FieldFrame id={id} label={label} hideLabel={hideLabel} hint={hint} error={error}>
      <span className="relative flex">
        <select
          id={id}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedIds(id, hint, error)}
          className={cx(
            fieldClass,
            'cursor-pointer appearance-none pr-12 font-semibold',
            error ? 'border-2 border-danger' : 'border-control',
            className,
          )}
          {...rest}
        >
          {children}
        </select>
        <ChevronDownIcon className="pointer-events-none absolute top-1/2 right-4 -translate-y-1/2 text-ink-2" />
      </span>
    </FieldFrame>
  )
}

function FieldFrame({
  id,
  label,
  hideLabel,
  hint,
  error,
  children,
}: {
  id: string
  label: string
  hideLabel: boolean
  hint?: ReactNode | undefined
  error?: string | undefined
  children: ReactNode
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <label
        htmlFor={id}
        className={cx(
          'text-label font-bold tracking-label text-ink-2 uppercase',
          hideLabel && 'sr-only',
        )}
      >
        {label}
      </label>
      {children}
      {hint && !error && (
        <span id={`${id}-hint`} className="text-caption text-ink-3">
          {hint}
        </span>
      )}
      {error && (
        <span id={`${id}-error`} className="flex items-start gap-2 text-body-sm text-danger">
          <ErrorIcon className="mt-0.5 shrink-0" />
          {error}
        </span>
      )}
    </div>
  )
}

const fieldClass =
  'box-border min-h-control-md w-full rounded-md border bg-surface-2 px-4 text-body text-ink-1 placeholder:text-ink-3 outline-none transition duration-120 focus:border-accent'

export function SearchField({
  label,
  shortcut = false,
  value,
  onClear,
  className,
  ...rest
}: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & {
  label: string
  shortcut?: boolean
  onClear?: () => void
}) {
  return (
    <label
      className={cx(
        'flex min-h-control-sm min-w-0 items-center gap-2.5 rounded-pill border border-control bg-surface-2 px-4 text-ink-2 has-[:focus-visible]:border-accent',
        className,
      )}
    >
      <SearchIcon />
      <span className="sr-only">{label}</span>
      <input
        type="search"
        aria-label={label}
        value={value}
        data-shortcut={shortcut ? 'search' : undefined}
        className="min-w-0 flex-1 border-0 bg-transparent text-body-sm text-ink-1 outline-none placeholder:text-ink-3"
        {...rest}
      />
      {onClear && value && (
        <button
          type="button"
          aria-label={label}
          className="inline-flex size-6 items-center justify-center rounded-full text-ink-2"
          onClick={onClear}
        >
          <CloseIcon size={15} />
        </button>
      )}
      {shortcut && (
        <kbd
          aria-hidden="true"
          className="rounded-sm border border-control px-1.5 font-mono text-caption text-ink-3"
        >
          /
        </kbd>
      )}
    </label>
  )
}

export function PasswordField({
  reveal = true,
  ...props
}: CommonFieldProps &
  Omit<ComponentPropsWithRef<'input'>, 'size' | 'type'> & { reveal?: boolean }) {
  const [shown, setShown] = useState(false)
  const { t } = useTranslation()
  return (
    <div className="relative">
      <TextField {...props} type={shown ? 'text' : 'password'} />
      {reveal && (
        <button
          type="button"
          aria-label={shown ? t('common.hidePassword') : t('common.showPassword')}
          aria-pressed={shown}
          className="absolute right-3 bottom-2 inline-flex min-h-control-sm items-center rounded-pill px-3 text-caption text-ink-2"
          onClick={() => {
            setShown((current) => !current)
          }}
        >
          <EyeIcon />
        </button>
      )}
    </div>
  )
}

export function Checkbox({
  label,
  checked,
  onChange,
  disabled = false,
}: {
  label: string
  checked?: boolean
  onChange?: (checked: boolean) => void
  disabled?: boolean
}) {
  return (
    <label className="flex min-h-control-sm cursor-pointer items-center gap-3 text-body">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange?.(event.currentTarget.checked)}
        className="size-5 accent-accent"
      />
      {label}
    </label>
  )
}

export function Switch({
  label,
  checked,
  onChange,
  disabled = false,
}: {
  label: string
  checked?: boolean
  onChange?: (checked: boolean) => void
  disabled?: boolean
}) {
  return (
    <label className="flex min-h-control-sm cursor-pointer items-center justify-between gap-4 text-body">
      <span>{label}</span>
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange?.(event.currentTarget.checked)}
        className="peer sr-only"
      />
      <span
        aria-hidden="true"
        className="relative h-7 w-12 rounded-pill bg-surface-3 transition peer-checked:bg-accent after:absolute after:top-1 after:left-1 after:size-5 after:rounded-full after:bg-ink-1 after:transition peer-checked:after:translate-x-5"
      />
    </label>
  )
}
