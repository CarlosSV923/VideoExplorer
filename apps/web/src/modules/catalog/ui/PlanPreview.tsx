import { useTranslation } from 'react-i18next'

import type { AssignPlan } from '../domain/types'
import { Banner, Button, EmptyState } from '@/shared/ui'

export function PlanPreview({
  plan,
  onSave,
  saveEnabled,
  saveReason,
  saving,
}: {
  plan: AssignPlan | null
  onSave: () => void
  saveEnabled: boolean
  saveReason?: string
  saving: boolean
}) {
  const { t } = useTranslation()
  return (
    <section className="flex flex-col gap-4 rounded-lg border border-line bg-surface p-5">
      <div>
        <h2 className="m-0 text-heading font-display">{t('assign.preview.title')}</h2>
        <p className="m-0 mt-1 text-body-sm text-ink-2">{t('assign.preview.body')}</p>
      </div>
      {!plan ? (
        <EmptyState title={t('assign.preview.emptyTitle')} body={t('assign.preview.emptyBody')} />
      ) : (
        <>
          {plan.videos.map((video) => (
            <div
              key={video.file}
              className="flex flex-col gap-1 border-b border-line pb-3 last:border-0"
            >
              <span className="break-all font-mono-file text-mono">
                {video.file || t('assign.video.loose')}
              </span>
              <span className="break-all font-mono-file text-caption text-ink-2">
                {video.finalPath ?? t('library.pendingPath')}
              </span>
              {video.exactCopy && (
                <span className="text-caption text-warning-ink">{t('assign.duplicate.exact')}</span>
              )}
            </div>
          ))}
          {plan.errors.map((issue) => (
            <Banner key={`${issue.file ?? 'entry'}-${issue.field}-${issue.code}`} tone="danger">
              {t(`assign.issue.${issue.code}`, { defaultValue: t('assign.issue.generic') })}
            </Banner>
          ))}
          {plan.warnings.map((issue) => (
            <Banner key={`${issue.file ?? 'entry'}-${issue.field}-${issue.code}`} tone="warning">
              {t(`assign.issue.${issue.code}`, { defaultValue: t('assign.issue.generic') })}
            </Banner>
          ))}
          <Button
            variant="primary"
            disabled={!saveEnabled}
            {...(saveReason ? { disabledReason: saveReason } : {})}
            loading={saving}
            onClick={onSave}
          >
            {t('assign.save')}
          </Button>
        </>
      )}
    </section>
  )
}
