import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '../application/ports'
import { useInvalidateCatalog, useLibraryTitle } from '../application/queries'
import type { Kind } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import { Banner, Button, ConfirmDialog, EmptyState, LinkButton, Tabs } from '@/shared/ui'
import { isAppError } from '@/shared/kernel/errors'
import { TitleHeader } from './TitleHeader'
import { VersionCard } from './VersionCard'
import { EpisodeList } from './EpisodeList'
import { CollectionInline } from './CollectionInline'
import { LoadingGrid } from './CatalogShared'
import {
  useLoadTitleMetadata,
  useRefreshTitleMetadata,
} from '@/modules/metadata/application/queries'

export function TitleDetailScreen({ id, kind }: { id: number; kind: Kind }) {
  const { t } = useTranslation()
  const query = useLibraryTitle(id)
  const ports = useCatalogPorts()
  const invalidateCatalog = useInvalidateCatalog()
  const [confirm, setConfirm] = useState<'unassigned' | 'trash' | null>(null)
  const [season, setSeason] = useState<number | null>(null)
  const refresh = useRefreshTitleMetadata(id)
  const loadMetadata = useLoadTitleMetadata(id)
  useEffect(() => {
    if (
      !query.data?.tmdbId ||
      loadMetadata.isPending ||
      loadMetadata.isError ||
      loadMetadata.isSuccess
    )
      return
    loadMetadata.mutate()
  }, [query.data?.tmdbId, loadMetadata])
  const move = useMutation({
    mutationFn: () => ports.moveTitleToUnassigned(id, { kind: 'title' }),
    onSuccess: () => {
      setConfirm(null)
      invalidateCatalog()
    },
  })
  const trash = useMutation({
    mutationFn: () => ports.trashTitleScope(id, { kind: 'title' }),
    onSuccess: () => {
      setConfirm(null)
      invalidateCatalog()
    },
  })
  if (query.isPending)
    return (
      <LibraryLayout>
        <div className="py-12">
          <LoadingGrid />
        </div>
      </LibraryLayout>
    )
  if (query.isError)
    return (
      <LibraryLayout>
        <div className="py-12">
          {isAppError(query.error, 'notFound') ? (
            <EmptyState
              title={t('missing.title')}
              body={t(kind === 'movie' ? 'missing.movie' : 'missing.series')}
              action={
                <LinkButton href={kind === 'movie' ? '/peliculas' : '/series'} variant="primary">
                  {t(kind === 'movie' ? 'missing.backMovies' : 'missing.backSeries')}
                </LinkButton>
              }
            />
          ) : (
            <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
          )}
        </div>
      </LibraryLayout>
    )
  const title = query.data
  const initialSeason =
    title.seasons.find((value) => value.number > 0 && value.episodes.length > 0)?.number ??
    title.seasons.find((value) => value.number === 0 && value.episodes.length > 0)?.number ??
    title.seasons.find((value) => value.number > 0)?.number ??
    title.seasons[0]?.number
  const selectedSeason = title.seasons.find((value) => value.number === (season ?? initialSeason))
  return (
    <LibraryLayout>
      <TitleHeader
        title={title}
        onRefresh={() => {
          refresh.mutate()
        }}
        refreshing={refresh.isPending}
      />
      {loadMetadata.isError && <Banner tone="warning">{t('library.refreshFailed')}</Banner>}
      {refresh.isError && <Banner tone="danger">{t('library.refreshFailed')}</Banner>}
      <div className="flex flex-col gap-8 py-8">
        <div className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            onClick={() => {
              setConfirm('unassigned')
            }}
          >
            {t('library.moveUnassigned')}
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              setConfirm('trash')
            }}
          >
            {t('library.trash')}
          </Button>
        </div>
        {kind === 'movie' && (
          <section>
            <h2 className="text-heading font-display">{t('library.files')}</h2>
            <div className="flex flex-col gap-3">
              {title.videos.map((video) => (
                <VersionCard key={video.id} titleId={id} video={video} kind="movie" />
              ))}
            </div>
          </section>
        )}
        {kind === 'series' && title.seasons.length > 0 && (
          <section>
            <h2 className="text-heading font-display">{t('library.seasons')}</h2>
            <Tabs
              label={t('library.seasons')}
              options={title.seasons.map((value) => ({
                value: String(value.number),
                label: (
                  <>
                    {value.number === 0
                      ? t('library.specials')
                      : t('library.season', { number: value.number })}{' '}
                    <span aria-hidden="true" className="text-ink-2">
                      {t('library.seasonProgress', {
                        present: value.episodes.filter((episode) => episode.status === 'present')
                          .length,
                        total: value.episodes.length,
                      })}
                    </span>
                  </>
                ),
              }))}
              value={selectedSeason ? String(selectedSeason.number) : ''}
              onChange={(value) => {
                setSeason(Number(value))
              }}
            />
            {selectedSeason ? (
              <EpisodeList title={title} season={selectedSeason} />
            ) : (
              <p className="m-0 text-body text-ink-2">{t('library.seasonEmpty')}</p>
            )}
          </section>
        )}
        {title.collection && <CollectionInline collection={title.collection} />}
      </div>
      {confirm === 'unassigned' && (
        <ConfirmDialog
          title={t('library.confirmMoveTitle')}
          confirmLabel={t('library.moveUnassigned')}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            move.mutate()
          }}
          busy={move.isPending}
        >
          {t('library.confirmMoveBody')}
        </ConfirmDialog>
      )}
      {confirm === 'trash' && (
        <ConfirmDialog
          tone="danger"
          title={t('library.confirmTrashTitle')}
          confirmLabel={t('library.trash')}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            trash.mutate()
          }}
          busy={trash.isPending}
        >
          {t('library.confirmTrashBody')}
        </ConfirmDialog>
      )}
    </LibraryLayout>
  )
}
