import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useHealth } from '../application/ports'
import { Button, Dialog, ErrorIcon } from '@/shared/ui'

export function SystemHealthBanner() {
  const { t } = useTranslation()
  const health = useHealth()
  const [open, setOpen] = useState(false)
  const failed = health.data?.checks.filter((check) => !check.ok) ?? []
  if (!failed.length) return null
  const technical = failed
    .map((check) => `${check.code ?? check.name}: ${check.message ?? ''}`)
    .join('\n')
  const library = failed.some((check) => check.code === 'library_not_writable')
  return (
    <>
      <div
        role="alert"
        className="sticky top-16 z-20 flex flex-wrap items-center justify-between gap-3 border-b border-danger bg-danger-surface px-[var(--page-x,16px)] py-3 text-danger-ink"
      >
        <span className="flex min-w-0 items-start gap-2">
          <ErrorIcon className="mt-0.5 shrink-0 text-danger" />
          <span>
            <strong>{library ? t('health.libraryTitle') : t('health.genericTitle')}</strong>{' '}
            {library ? t('health.libraryBody') : t('health.genericBody')}
          </span>
        </span>
        <Button
          variant="danger"
          size="sm"
          onClick={() => {
            setOpen(true)
          }}
        >
          {t('health.details')}
        </Button>
      </div>
      {open && (
        <Dialog
          title={t('health.detailsTitle')}
          onClose={() => {
            setOpen(false)
          }}
          actions={
            <>
              <Button
                onClick={() => {
                  setOpen(false)
                }}
              >
                {t('common.close')}
              </Button>
              <Button
                variant="primary"
                onClick={() => {
                  const clipboard = (
                    navigator as unknown as {
                      clipboard?: { writeText(value: string): Promise<void> }
                    }
                  ).clipboard
                  if (clipboard) void clipboard.writeText(technical)
                }}
              >
                {t('health.copy')}
              </Button>
            </>
          }
        >
          <ul className="m-0 flex list-none flex-col gap-3 p-0">
            {failed.map((check) => (
              <li key={check.name} className="flex flex-col gap-1">
                <strong>{check.code ? t(`health.code.${check.code}` as never) : check.name}</strong>
                <span className="font-mono text-body-sm text-ink-2">
                  {check.message ?? t('health.noMessage')}
                </span>
                {(check.path || check.uid !== undefined || check.gid !== undefined) && (
                  <span className="font-mono text-caption text-ink-2">
                    {[
                      check.path,
                      check.uid === undefined ? undefined : `uid=${String(check.uid)}`,
                      check.gid === undefined ? undefined : `gid=${String(check.gid)}`,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </Dialog>
      )}
    </>
  )
}
