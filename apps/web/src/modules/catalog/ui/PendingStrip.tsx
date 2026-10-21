import { useTranslation } from 'react-i18next'

import type { LibraryTitleSummary } from '../domain/types'
import { AppLink } from '@/shared/ui/AppLink'

export function PendingStrip({
  titles,
  unassignedCount,
}: {
  titles: LibraryTitleSummary[]
  unassignedCount?: number
}) {
  const { t } = useTranslation()
  const unlinked = titles.filter((title) => !title.tmdbId).length
  const incomplete = titles.filter((title) => (title.missingEpisodes ?? 0) > 0).length
  return (
    <section aria-label={t('library.pending')} className="flex flex-wrap gap-3">
      {unassignedCount !== undefined && unassignedCount > 0 && (
        <AppLink
          href="/unassigned"
          className="rounded-md border border-warning bg-warning-surface px-4 py-3 text-warning-ink no-underline"
        >
          {t('library.pendingUnassigned', { count: unassignedCount })}
        </AppLink>
      )}
      {incomplete > 0 && (
        <AppLink
          href="/series?incomplete=1"
          className="rounded-md border border-warning bg-warning-surface px-4 py-3 text-warning-ink no-underline"
        >
          {t('library.pendingIncomplete', { count: incomplete })}
        </AppLink>
      )}
      {unlinked > 0 && (
        <div className="rounded-md border border-warning bg-warning-surface px-4 py-3 text-warning-ink">
          <span className="block">{t('library.pendingUnlinked', { count: unlinked })}</span>
          <span className="block text-body-sm">
            {t('library.pendingUnlinkedBody', {
              titles: titles
                .filter((title) => !title.tmdbId)
                .map((title) => title.title)
                .join(', '),
            })}
          </span>
        </div>
      )}
    </section>
  )
}
