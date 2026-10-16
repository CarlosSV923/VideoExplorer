import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '../application/ports'
import { useCommitTitleEdit, useLibraryTitle } from '../application/queries'
import type { Kind } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import { useTitleSearch } from '@/modules/metadata/application/queries'
import { isAppError } from '@/shared/kernel/errors'
import { Banner, Button, EmptyState, PageHeader, SegmentedControl, TextField } from '@/shared/ui'
import { LoadingGrid } from './CatalogShared'

export function EditScreen({ id }: { id: number }) {
  const { t } = useTranslation()
  const query = useLibraryTitle(id)
  const ports = useCatalogPorts()
  const [term, setTerm] = useState('')
  const [choice, setChoice] = useState<{
    id: number
    kind: Kind
    title: string
    year?: number
  } | null>(null)
  const search = useTitleSearch(term)
  const hits = search.data ? [...search.data.library, ...search.data.tmdb] : []
  const [planned, setPlanned] = useState<Awaited<ReturnType<typeof ports.planTitleEdit>> | null>(
    null,
  )
  const commit = useCommitTitleEdit(id)
  if (query.isPending)
    return (
      <LibraryLayout hideMobileNav>
        <div className="py-12">
          <LoadingGrid />
        </div>
      </LibraryLayout>
    )
  if (query.isError)
    return (
      <LibraryLayout hideMobileNav>
        <div className="py-12">
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )
  const title = query.data
  const request = () => ({
    revision: planned?.revision ?? '',
    title: choice
      ? { source: 'tmdb' as const, tmdbId: choice.id, kind: choice.kind }
      : { source: 'library' as const, libraryTitleId: id },
    videos: title.videos.map((video) => ({
      fileId: video.id,
      ...(video.season !== undefined
        ? {
            season: video.season,
            chapter: video.chapter ?? 1,
            lastChapter: video.lastChapter ?? video.chapter ?? 1,
          }
        : { part: video.part }),
      version: video.version,
    })),
    subtitles: title.videos.flatMap((video) =>
      video.subtitles.map((subtitle) => ({
        fileId: subtitle.id,
        language: subtitle.language,
        forced: subtitle.forced,
        sdh: subtitle.sdh,
      })),
    ),
  })
  const makePlan = () => {
    void ports
      .planTitleEdit(id, request())
      .then(setPlanned)
      .catch(() => undefined)
  }
  const save = () => {
    if (planned) commit.mutate({ ...request(), revision: planned.revision })
  }
  return (
    <LibraryLayout hideMobileNav>
      <PageHeader
        back={
          <a href={`/${title.kind === 'movie' ? 'peliculas' : 'series'}/${String(id)}`}>
            {title.title}
          </a>
        }
        title={title.tmdbId ? t('library.edit') : t('library.link')}
        subtitle={t('library.editSubtitle')}
      />
      <div className="grid gap-8 py-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section className="flex flex-col gap-5 rounded-lg border border-line bg-surface p-5">
          <SegmentedControl
            label={t('library.type')}
            options={[
              { value: 'movie', label: t('library.movies') },
              { value: 'series', label: t('library.seriesPlural') },
            ]}
            value={choice?.kind ?? title.kind}
            onChange={() => {}}
          />
          <TextField
            label={t('library.tmdbMatch')}
            value={term}
            onChange={(event) => {
              setTerm(event.currentTarget.value)
            }}
            placeholder={title.title}
          />
          {hits.map((hit) => (
            <button
              key={`${hit.kind}-${String(hit.tmdbId ?? hit.libraryTitleId)}`}
              type="button"
              className="flex items-center gap-3 rounded-md border border-control p-3 text-left"
              onClick={() => {
                if (hit.tmdbId)
                  setChoice({
                    id: hit.tmdbId,
                    kind: hit.kind,
                    title: hit.title,
                    ...(hit.year !== undefined ? { year: hit.year } : {}),
                  })
              }}
            >
              <span className="size-12 shrink-0 rounded bg-surface-2" />
              <span>
                <strong>{hit.title}</strong>
                <small className="block text-ink-2">{String(hit.year ?? '')}</small>
              </span>
            </button>
          ))}
          <label className="flex min-h-control-sm items-center gap-3 text-body">
            <input
              type="checkbox"
              defaultChecked={title.idTag}
              disabled={!choice && !title.tmdbId}
              className="size-5 accent-accent"
            />
            {t('library.idTag')}
          </label>
          <Button variant="secondary" onClick={makePlan}>
            {t('library.preview')}
          </Button>
        </section>
        <section className="rounded-lg border border-line bg-surface p-5">
          <h2 className="m-0 text-heading font-display">{t('library.previewTitle')}</h2>
          {planned ? (
            <>
              <p className="font-mono-file text-mono">{planned.folder}</p>
              {planned.videos.map((video) => (
                <p key={video.fileId} className="font-mono-file text-mono">
                  {video.finalPath ?? t('library.pendingPath')}
                </p>
              ))}
              {planned.errors.map((error) => (
                <Banner key={`${error.file ?? 'title'}-${error.field}`} tone="danger">
                  {t(`library.planError.${error.code}`, { defaultValue: error.code })}
                </Banner>
              ))}
              <Button variant="primary" disabled={!planned.valid} onClick={save}>
                {t('library.save')}
              </Button>
            </>
          ) : (
            <p className="text-body text-ink-2">{t('library.previewEmpty')}</p>
          )}
        </section>
      </div>
      {commit.isError && (
        <Banner tone="danger">
          {isAppError(commit.error, 'conflict') && commit.error.code === 'plan_changed'
            ? t('library.planError.plan_changed')
            : t('library.planChanged')}
        </Banner>
      )}
    </LibraryLayout>
  )
}
