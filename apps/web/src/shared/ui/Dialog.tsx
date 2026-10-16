import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { useAction } from '../input'
import { tabbables } from '../input/spatial'
import { Button } from './Button'
import { CloseIcon } from './icons'
import { cx } from './cx'

interface DialogProps {
  title: string
  onClose: () => void
  actions?: ReactNode
  initialFocus?: 'action' | 'cancel' | 'first' | 'field'
  subtitle?: string
  size?: 'md' | 'lg'
  placement?: 'center' | 'bottom'
  children?: ReactNode
}

export function Dialog({
  title,
  onClose,
  actions,
  initialFocus = 'action',
  subtitle,
  size = 'md',
  placement = 'center',
  children,
}: DialogProps) {
  const titleId = useId()
  const box = useRef<HTMLDivElement>(null)
  const [host] = useState(() => document.createElement('div'))
  useAction('back', () => {
    onClose()
    return true
  })

  useLayoutEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    document.body.append(host)
    const siblings = Array.from(document.body.children).filter(
      (element) => element !== host && !element.hasAttribute('inert'),
    )
    siblings.forEach((element) => {
      element.setAttribute('inert', '')
    })
    return () => {
      siblings.forEach((element) => {
        element.removeAttribute('inert')
      })
      host.remove()
      opener?.focus()
    }
  }, [host])

  useEffect(() => {
    const items = box.current ? tabbables(box.current) : []
    const buttons = items.filter((element) => element instanceof HTMLButtonElement)
    const actionButtons = buttons.filter((button) => !button.hasAttribute('data-dialog-close'))
    const field = items.find(
      (element) =>
        element instanceof HTMLInputElement &&
        (element.type === 'text' || element.type === 'search'),
    )
    const target =
      initialFocus === 'field'
        ? (field ?? items[0])
        : initialFocus === 'cancel'
          ? (actionButtons[0] ?? items[0])
          : initialFocus === 'first'
            ? items[0]
            : (buttons.at(-1) ?? items[0])
    target?.focus()
  }, [initialFocus])

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Tab' || !box.current) return
    const items = tabbables(box.current)
    const first = items[0]
    const last = items.at(-1)
    if (!first || !last) return
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  return createPortal(
    <div
      className={cx(
        'fixed inset-0 z-50 flex bg-overlay p-4 sm:p-6',
        placement === 'bottom' ? 'items-end' : 'items-center justify-center',
      )}
    >
      <div
        ref={box}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onKeyDown={onKeyDown}
        className={cx(
          'flex max-h-full w-full flex-col gap-5 overflow-y-auto border border-control bg-surface p-6',
          placement === 'bottom' ? 'animate-sheet-in rounded-t-xl' : 'animate-dialog-in rounded-xl',
          size === 'lg' ? 'max-w-[720px]' : 'max-w-[560px]',
        )}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 id={titleId} className="m-0 text-title font-display">
              {title}
            </h2>
            {subtitle && <p className="m-0 mt-1 text-body-sm text-ink-2">{subtitle}</p>}
          </div>
          <button
            type="button"
            aria-label={title}
            data-dialog-close
            className="inline-flex size-control-sm shrink-0 items-center justify-center rounded-full text-ink-2 hover:bg-hover"
            onClick={onClose}
          >
            <CloseIcon />
          </button>
        </div>
        {children && <div className="flex flex-col gap-4 text-body-lg">{children}</div>}
        {actions && <div className="flex flex-wrap justify-end gap-3">{actions}</div>}
      </div>
    </div>,
    host,
  )
}

export function ConfirmDialog({
  title,
  confirmLabel,
  onConfirm,
  onCancel,
  tone = 'primary',
  busy = false,
  children,
}: {
  title: string
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
  tone?: 'primary' | 'danger'
  busy?: boolean
  children?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Dialog
      title={title}
      onClose={onCancel}
      initialFocus={tone === 'danger' ? 'cancel' : 'action'}
      actions={
        <>
          <Button onClick={onCancel}>{t('common.cancel')}</Button>
          <Button
            variant={tone === 'danger' ? 'danger-solid' : 'primary'}
            loading={busy}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {children}
    </Dialog>
  )
}

export function Sheet({
  title,
  open = true,
  onClose,
  children,
  actions,
}: {
  title: string
  open?: boolean
  onClose: () => void
  children: ReactNode
  actions?: ReactNode
}) {
  if (!open) return null
  return (
    <Dialog title={title} onClose={onClose} actions={actions} size="lg" placement="bottom">
      <div className="-mx-6 -mb-6 border-t border-line px-6 pt-4">
        <span aria-hidden="true" className="mx-auto mb-4 block h-1 w-12 rounded-pill bg-control" />
        {children}
      </div>
    </Dialog>
  )
}

export function SidePanel({
  title,
  open = true,
  onClose,
  children,
}: {
  title: string
  open?: boolean
  onClose: () => void
  children: ReactNode
}) {
  useAction(
    'back',
    () => {
      if (!open) return false
      onClose()
      return true
    },
    open,
  )
  if (!open) return null
  return (
    <aside
      aria-label={title}
      className="fixed inset-y-0 right-0 z-40 flex w-full max-w-[420px] animate-panel-in flex-col border-l border-line bg-surface p-6 shadow-xl"
    >
      <div className="flex items-center justify-between gap-4">
        <h2 className="m-0 text-title font-display">{title}</h2>
        <button
          type="button"
          aria-label={title}
          className="inline-flex size-control-sm items-center justify-center rounded-full hover:bg-hover"
          onClick={onClose}
        >
          <CloseIcon />
        </button>
      </div>
      <div className="mt-6 min-h-0 flex-1 overflow-y-auto">{children}</div>
    </aside>
  )
}
