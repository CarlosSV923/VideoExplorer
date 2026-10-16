import { useTranslation } from 'react-i18next'

import type { DuplicateDecision, PlanVideo } from '../domain/types'
import { SegmentedControl, StatusChip } from '@/shared/ui'

export function DuplicateChoice({
  plan,
  value,
  onChange,
  error,
}: {
  plan: PlanVideo | undefined
  value?: DuplicateDecision
  onChange: (value: DuplicateDecision) => void
  error?: string | undefined
}) {
  const { t } = useTranslation()
  if (!plan?.duplicates.length) return null
  const allowed = plan.allowed
  return (
    <div className="flex flex-col gap-3 rounded-md border border-warning bg-warning-surface p-3">
      <div className="flex flex-wrap items-center gap-2">
        <strong>{t('assign.duplicate.title')}</strong>
        {plan.exactCopy && <StatusChip tone="warning">{t('assign.duplicate.exact')}</StatusChip>}
      </div>
      {plan.duplicates.map((duplicate) => (
        <p
          key={duplicate.rel}
          className="m-0 break-all font-mono-file text-caption text-warning-ink"
        >
          {duplicate.rel}
        </p>
      ))}
      <SegmentedControl
        label={t('assign.duplicate.choice')}
        options={allowed.map((option) => ({
          value: option,
          label: t(`assign.duplicate.${option}`),
        }))}
        value={value ?? plan.proposal ?? allowed[0] ?? 'skip'}
        onChange={onChange}
      />
      {error && <p className="m-0 text-body-sm text-danger">{error}</p>}
    </div>
  )
}
