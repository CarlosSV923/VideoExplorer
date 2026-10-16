import { useTranslation } from 'react-i18next'

import type { LibraryTitle } from '../domain/types'
import { StatusChip } from '@/shared/ui'
import { formatDate } from '@/shared/i18n/format'
import { useLanguage } from '@/shared/i18n/hooks'
import { VersionCard } from './VersionCard'

export function EpisodeList({
  title,
  season,
}: {
  title: LibraryTitle
  season: LibraryTitle['seasons'][number]
}) {
  const { t } = useTranslation()
  const language = useLanguage()
  return (
    <div className="flex flex-col gap-3">
      {season.episodes.map((episode) => (
        <article
          key={episode.id}
          className={`grid gap-4 rounded-lg border border-line p-3 md:grid-cols-[224px_1fr] ${episode.status !== 'present' ? 'bg-surface-missing' : 'bg-surface'}`}
        >
          <div className={episode.status === 'present' ? '' : 'brightness-55'}>
            {episode.thumbnail ? (
              <img
                src={episode.thumbnail}
                alt=""
                className="aspect-video w-full rounded-md object-cover"
              />
            ) : (
              <div className="aspect-video rounded-md bg-surface-2" />
            )}
          </div>
          <div className="flex min-w-0 flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="m-0 text-subheading font-display">
                {t('library.episodeNumber', { number: episode.number })} ·{' '}
                {episode.title || t('library.untitledEpisode')}
              </h3>
              <StatusChip
                tone={
                  episode.status === 'present'
                    ? 'success'
                    : episode.status === 'missing'
                      ? 'warning'
                      : 'info'
                }
              >
                {t(`library.episode.${episode.status}`, { defaultValue: episode.status })}
              </StatusChip>
            </div>
            {episode.airDate && (
              <time dateTime={episode.airDate} className="text-body-sm text-ink-2">
                {formatDate(new Date(episode.airDate), language)}
              </time>
            )}
            <p className="m-0 text-body-sm text-ink-2">{episode.overview}</p>
            {episode.files.map((file) => (
              <VersionCard key={file.id} titleId={title.id} video={file} kind="series" />
            ))}
          </div>
        </article>
      ))}
    </div>
  )
}
