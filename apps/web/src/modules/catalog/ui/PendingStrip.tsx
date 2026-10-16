import { useTranslation } from 'react-i18next'

import type { LibraryTitleSummary } from '../domain/types'

export function PendingStrip({ titles }: { titles: LibraryTitleSummary[] }) {
  const { t } = useTranslation()
  const unlinked = titles.filter((title) => !title.tmdbId).length
  const incomplete = titles.filter((title) => (title.missingEpisodes ?? 0) > 0).length
  return (
    <section aria-label={t('library.pending')} className="flex flex-wrap gap-3">
      <a
        href="/no-asignados"
        className="rounded-md border border-warning bg-warning-surface px-4 py-3 text-warning-ink no-underline"
      >
        {t('library.pendingUnassigned')}
      </a>
      {incomplete > 0 && (
        <a
          href="/series?incompletas=1"
          className="rounded-md border border-warning bg-warning-surface px-4 py-3 text-warning-ink no-underline"
        >
          {t('library.pendingIncomplete', { count: incomplete })}
        </a>
      )}
      {unlinked > 0 && (
        <span className="rounded-md border border-warning bg-warning-surface px-4 py-3 text-warning-ink">
          {t('library.pendingUnlinked', { count: unlinked })}
        </span>
      )}
    </section>
  )
}
