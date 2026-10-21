import { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { titleChoiceFromHit } from '../application/assignment'
import { useCatalogPorts } from '../application/ports'
import { useCommitTitleEdit, useInvalidateCatalog, useLibraryTitle } from '../application/queries'
import type {
  EditPlan,
  EditRequest,
  IDTagRequest,
  LibraryTitle,
  PlanIssue,
  TitleChoice,
  TitleHit,
} from '../domain/types'
import type { Kind } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import { useMetadataPorts } from '@/modules/metadata/application/ports'
import { useTitleSearch } from '@/modules/metadata/application/queries'
import { isAppError } from '@/shared/kernel/errors'
import { useFormat } from '@/shared/i18n/hooks'
import {
  Banner,
  Button,
  Checkbox,
  Dialog,
  EmptyState,
  NumberField,
  PageHeader,
  SegmentedControl,
  Select,
  Switch,
  TextField,
} from '@/shared/ui'
import { LoadingGrid } from './CatalogShared'
import { TitleHitOptions } from './TitleHitOptions'
import { AppLink } from '@/shared/ui/AppLink'

const languages = [
  ['es-419', 'Español (Latinoamérica)'],
  ['es', 'Español'],
  ['en', 'English'],
  ['pt-BR', 'Português (Brasil)'],
  ['fr', 'Français'],
  ['ja', '日本語'],
] as const

function currentChoice(title: LibraryTitle): TitleChoice {
  if (title.tmdbId !== undefined) return { source: 'tmdb', kind: title.kind, tmdbId: title.tmdbId }
  return {
    source: 'free',
    kind: title.kind,
    title: title.discTitle,
    ...(title.year > 0 ? { year: title.year } : {}),
  }
}

function videoRequest(
  video: LibraryTitle['videos'][number],
  kind: Kind,
): EditRequest['videos'][number] {
  const base = { fileId: video.id, version: video.version }
  if (kind === 'movie') return video.part ? { ...base, part: video.part } : base
  const chapter = video.chapter ?? 1
  const last = video.lastChapter ?? chapter
  return {
    ...base,
    season: video.season ?? 0,
    chapter,
    ...(last > chapter ? { lastChapter: last } : {}),
    ...(video.chapterTitle ? { chapterTitle: video.chapterTitle } : {}),
  }
}

type Translate = (key: string, options?: Record<string, unknown>) => string

function issueMessage(t: Translate, issue?: PlanIssue): string | undefined {
  return issue
    ? t(`library.issue.${issue.code}`, { defaultValue: t('library.issue.generic') })
    : undefined
}

export function EditScreen({ id }: { id: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const format = useFormat()
  const translate = t as unknown as Translate
  const query = useLibraryTitle(id)
  const ports = useCatalogPorts()
  const metadata = useMetadataPorts()
  const [term, setTerm] = useState('')
  const [openMatch, setOpenMatch] = useState(false)
  const [choice, setChoice] = useState<TitleChoice | null>(null)
  const [selectedHit, setSelectedHit] = useState<TitleHit | null>(null)
  const [freeTitle, setFreeTitle] = useState('')
  const [freeYear, setFreeYear] = useState('')
  const [kind, setKind] = useState<Kind>('movie')
  const [idTag, setIdTag] = useState(false)
  const [videos, setVideos] = useState<EditRequest['videos']>([])
  const [subtitles, setSubtitles] = useState<EditRequest['subtitles']>([])
  const [editDirty, setEditDirty] = useState(false)
  const [planned, setPlanned] = useState<Awaited<ReturnType<typeof ports.planTitleEdit>> | null>(
    null,
  )
  const [planMode, setPlanMode] = useState<'edit' | 'id-tag'>('edit')
  const [planError, setPlanError] = useState<unknown>(null)
  const commit = useCommitTitleEdit(id)
  const invalidateCatalog = useInvalidateCatalog()
  const title = query.data
  const search = useTitleSearch(term, title?.year)
  const taggedQuery = title?.videos
    .map((video) => video.path.match(/\[(?:tmdbid-(\d+)|imdbid-(tt\d+))\]|\{tmdb-(\d+)\}/i))
    .find(Boolean)
  const tagTerm = taggedQuery
    ? taggedQuery[2]
      ? `[imdbid-${taggedQuery[2]}]`
      : `[tmdbid-${taggedQuery[1] ?? taggedQuery[3] ?? ''}]`
    : ''
  const tagSearch = useTitleSearch(tagTerm)
  const libraryHits = search.data?.library.filter((hit) => hit.libraryTitleId !== id) ?? []
  const taggedHits = tagSearch.data?.tmdb.filter((hit) => hit.libraryTitleId !== id) ?? []
  const tmdbHits = [
    ...taggedHits,
    ...(search.data?.tmdb.filter(
      (hit) =>
        hit.libraryTitleId !== id &&
        !taggedHits.some(
          (preferred) => preferred.kind === hit.kind && preferred.tmdbId === hit.tmdbId,
        ),
    ) ?? []),
  ]
  const hits = [...libraryHits, ...tmdbHits]

  useEffect(() => {
    if (!title) return
    // eslint-disable-next-line react-hooks/set-state-in-effect -- hydrate the form from the loaded title
    setKind(title.kind)
    setIdTag(title.idTag)
    setVideos(title.videos.map((video) => videoRequest(video, title.kind)))
    setSubtitles(
      title.videos.flatMap((video) =>
        video.subtitles.map((subtitle) => ({
          fileId: subtitle.id,
          language: subtitle.language,
          forced: subtitle.forced,
          sdh: subtitle.sdh,
        })),
      ),
    )
    setChoice(null)
    setSelectedHit(null)
    setTerm(title.title)
    setEditDirty(false)
    setPlanned(null)
  }, [title])

  const target = title ? { ...(choice ?? currentChoice(title)), kind } : null
  const matchTitle = selectedHit?.title ?? (target?.source === 'free' ? target.title : title?.title)
  const matchYear = selectedHit?.year ?? target?.year ?? title?.year
  const matchTMDB = target?.tmdbId ?? title?.tmdbId
  const idTagDirty = Boolean(title && idTag !== title.idTag)
  const editRequest = (): EditRequest => ({
    revision: planned?.revision ?? '',
    title: target ?? { source: 'library', libraryTitleId: id },
    videos,
    subtitles,
  })
  const idTagRequest = (): IDTagRequest => ({
    revision: planned?.revision ?? '',
    idTag,
    videos: [],
    labels: [],
  })

  const resetPlan = () => {
    setPlanned(null)
    if (title && idTag !== title.idTag) setIdTag(title.idTag)
  }
  const updateEdit = (next: (current: EditRequest['videos']) => EditRequest['videos']) => {
    setVideos(next)
    setEditDirty(true)
    resetPlan()
  }
  const updateSubtitle = (fileId: number, patch: Partial<EditRequest['subtitles'][number]>) => {
    setSubtitles((current) =>
      current.map((subtitle) =>
        subtitle.fileId === fileId ? { ...subtitle, ...patch } : subtitle,
      ),
    )
    setEditDirty(true)
    resetPlan()
  }
  const makePlan = () => {
    setPlanError(null)
    if (idTagDirty && !editDirty) {
      setPlanMode('id-tag')
      void ports.planTitleIDTag(id, idTagRequest()).then(setPlanned).catch(setPlanError)
      return
    }
    setPlanMode('edit')
    void ports.planTitleEdit(id, editRequest()).then(setPlanned).catch(setPlanError)
  }
  const save = () => {
    if (!planned?.valid) return
    setPlanError(null)
    if (planMode === 'id-tag') {
      void ports
        .commitTitleIDTag(id, { ...idTagRequest(), revision: planned.revision })
        .then(() => {
          invalidateCatalog()
          void navigate({
            to: `/${kind === 'movie' ? 'movies' : 'series'}/${String(id)}` as never,
          })
        })
        .catch(setPlanError)
      return
    }
    commit.mutate({ ...editRequest(), revision: planned.revision }, { onError: setPlanError })
  }
  const chooseHit = (hit: TitleHit) => {
    setSelectedHit(hit)
    setChoice(titleChoiceFromHit(hit))
    setKind(hit.kind)
    setEditDirty(true)
    setPlanned(null)
    if (title && idTag !== title.idTag) setIdTag(title.idTag)
    setOpenMatch(false)
  }
  const useFree = () => {
    if (!freeTitle.trim()) return
    setSelectedHit(null)
    setChoice({
      source: 'free',
      kind,
      title: freeTitle.trim(),
      ...(freeYear.trim() ? { year: Number(freeYear) } : {}),
    })
    setEditDirty(true)
    setPlanned(null)
    setOpenMatch(false)
  }
  const planIssue = (file: string, field: string) =>
    issueMessage(
      translate,
      planned?.errors.find(
        (issue) =>
          (issue.file === file || issue.target === file) &&
          (issue.field === field ||
            (issue.field === 'place' &&
              ['part', 'season', 'chapter', 'lastChapter'].includes(field))),
      ),
    )
  const fileIssue = (file: string) =>
    issueMessage(
      translate,
      planned?.errors.find(
        (issue) =>
          (issue.file === file || issue.target === file) &&
          ['file', 'video', 'subtitle'].includes(issue.field),
      ),
    )

  if (query.isPending)
    return (
      <LibraryLayout hideMobileNav>
        <div className="py-12">
          <LoadingGrid />
        </div>
      </LibraryLayout>
    )
  if (query.isError || !title)
    return (
      <LibraryLayout hideMobileNav>
        <div className="py-12">
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )

  return (
    <LibraryLayout hideMobileNav>
      <PageHeader
        back={
          <AppLink href={`/${title.kind === 'movie' ? 'movies' : 'series'}/${String(id)}`}>
            {title.title}
          </AppLink>
        }
        title={title.tmdbId ? t('library.edit') : t('library.link')}
        subtitle={t('library.editSubtitle')}
        actions={
          <Button
            onClick={() => {
              window.history.back()
            }}
          >
            {t('common.cancel')}
          </Button>
        }
      />
      {Boolean(planError) && (
        <div className="page-padding pt-5">
          <Banner tone="danger">
            {isAppError(planError, 'conflict') && planError.code === 'plan_changed'
              ? translate('library.planError.plan_changed')
              : translate('errors.loadBody')}
          </Banner>
        </div>
      )}
      <div className="grid gap-8 py-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section className="flex flex-col gap-5">
          <section className="flex flex-wrap items-center gap-4 rounded-lg border border-line bg-surface p-4">
            {selectedHit?.posterFile || title.poster ? (
              <img
                src={
                  selectedHit?.posterFile
                    ? metadata.imageUrl('poster', selectedHit.posterFile)
                    : title.poster
                }
                alt=""
                className="size-16 shrink-0 rounded object-cover"
              />
            ) : (
              <span className="size-16 shrink-0 rounded bg-surface-2" aria-hidden="true" />
            )}
            <div className="min-w-0 flex-1">
              <h2 className="m-0 text-label font-bold tracking-label text-ink-2 uppercase">
                {t('library.currentMatch')}
              </h2>
              <p className="m-0 text-subheading font-bold">
                {matchTitle} {matchYear ? `(${String(matchYear)})` : ''}
              </p>
              <p className="m-0 text-body-sm text-ink-2">
                {kind === 'movie' ? t('library.movie') : t('library.series')}{' '}
                {t('common.separator')}{' '}
                {matchTMDB ? `TMDB ${String(matchTMDB)}` : t('library.unlinked')}
              </p>
            </div>
            <div className="w-full sm:max-w-sm">
              <TextField
                label={t('library.tmdbMatch')}
                value={term}
                onFocus={() => {
                  setOpenMatch(true)
                }}
                onChange={(event) => {
                  setTerm(event.currentTarget.value)
                }}
              />
            </div>
            <Button
              onClick={() => {
                setOpenMatch(true)
              }}
            >
              {t('library.changeMatch')}
            </Button>
          </section>
          {!openMatch && hits.length > 0 && (
            <div className="flex flex-col gap-4 rounded-lg border border-line bg-surface p-3">
              <TitleHitOptions
                key={term}
                library={libraryHits}
                tmdb={tmdbHits}
                selected={selectedHit}
                onPick={chooseHit}
              />
            </div>
          )}
          {planned?.errors
            .filter((issue) => !issue.file && ['title', 'year', 'kind'].includes(issue.field))
            .map((issue) => (
              <p key={issue.code} className="m-0 text-body-sm text-danger">
                {issueMessage(translate, issue)}
              </p>
            ))}
          <section className="flex flex-wrap items-center gap-4 rounded-lg border border-line bg-surface p-4">
            <div className="min-w-0 flex-1">
              <h2 className="m-0 text-subheading font-bold">{t('library.idTag')}</h2>
              <p className="m-0 text-body-sm text-ink-2">{t('library.idTagDescription')}</p>
            </div>
            <Switch
              label={matchTMDB ? `[tmdbid-${String(matchTMDB)}]` : t('library.unlinked')}
              checked={idTag}
              disabled={!matchTMDB}
              onChange={(checked) => {
                setIdTag(checked)
                setPlanned(null)
              }}
            />
          </section>
          <section className="flex flex-wrap items-end gap-4 rounded-lg border border-line bg-surface p-5">
            <SegmentedControl
              label={t('library.type')}
              options={[
                { value: 'movie', label: t('library.movie') },
                { value: 'series', label: t('library.series') },
              ]}
              value={kind}
              onChange={(next) => {
                setKind(next)
                setChoice((current) => (current ? { ...current, kind: next } : current))
                setVideos((current) =>
                  current.map((item) => {
                    if (next === 'movie') {
                      const movie = { ...item }
                      delete movie.season
                      delete movie.chapter
                      delete movie.lastChapter
                      delete movie.chapterTitle
                      return movie
                    }
                    const series = { ...item }
                    delete series.part
                    return {
                      ...series,
                      season: series.season ?? 0,
                      chapter: series.chapter ?? 1,
                    }
                  }),
                )
                setEditDirty(true)
                resetPlan()
              }}
            />
            <Button variant="secondary" onClick={makePlan}>
              {t('library.preview')}
            </Button>
          </section>
          <section className="flex flex-col gap-4">
            <h2 className="m-0 text-heading font-display">{t('library.files')}</h2>
            {title.videos.map((video) => {
              const value =
                videos.find((item) => item.fileId === video.id) ?? videoRequest(video, kind)
              const error = (field: string) => planIssue(video.path, field)
              return (
                <article
                  key={video.id}
                  className="flex w-full min-w-0 flex-col gap-4 rounded-lg border border-line bg-surface p-4"
                >
                  <div className="flex min-w-0 flex-wrap items-end gap-4">
                    <div className="min-w-0 max-w-full flex-1">
                      <p className="m-0 break-all [overflow-wrap:anywhere] font-mono-file text-mono">
                        {video.path}
                      </p>
                      {fileIssue(video.path) && (
                        <p className="m-0 text-body-sm text-danger">{fileIssue(video.path)}</p>
                      )}
                      <p className="m-0 text-body-sm text-ink-2">{format.size(video.size)}</p>
                    </div>
                    <TextField
                      label={t('library.versionLabel')}
                      value={value.version ?? ''}
                      {...(error('version') ? { error: error('version') } : {})}
                      onChange={(event) => {
                        updateEdit((current) =>
                          current.map((item) =>
                            item.fileId === video.id
                              ? { ...item, version: event.currentTarget.value }
                              : item,
                          ),
                        )
                      }}
                    />
                  </div>
                  {kind === 'movie' ? (
                    <NumberField
                      label={t('assign.video.part')}
                      min={1}
                      value={value.part ?? ''}
                      {...(error('part') ? { error: error('part') } : {})}
                      onChange={(event) => {
                        updateEdit((current) =>
                          current.map((item) => {
                            if (item.fileId !== video.id) return item
                            const next = { ...item }
                            if (event.currentTarget.value)
                              next.part = Number(event.currentTarget.value)
                            else delete next.part
                            return next
                          }),
                        )
                      }}
                    />
                  ) : (
                    <div className="grid gap-3 sm:grid-cols-3">
                      <NumberField
                        label={t('assign.video.season')}
                        min={0}
                        value={value.season ?? ''}
                        {...(error('season') ? { error: error('season') } : {})}
                        onChange={(event) => {
                          updateEdit((current) =>
                            current.map((item) =>
                              item.fileId === video.id
                                ? { ...item, season: Number(event.currentTarget.value) }
                                : item,
                            ),
                          )
                        }}
                      />
                      <NumberField
                        label={t('assign.video.chapter')}
                        min={1}
                        value={value.chapter ?? ''}
                        {...(error('chapter') ? { error: error('chapter') } : {})}
                        onChange={(event) => {
                          updateEdit((current) =>
                            current.map((item) =>
                              item.fileId === video.id
                                ? { ...item, chapter: Number(event.currentTarget.value) }
                                : item,
                            ),
                          )
                        }}
                      />
                      <NumberField
                        label={t('assign.video.lastChapter')}
                        min={1}
                        value={value.lastChapter ?? value.chapter ?? ''}
                        {...(error('lastChapter') ? { error: error('lastChapter') } : {})}
                        onChange={(event) => {
                          updateEdit((current) =>
                            current.map((item) => {
                              if (item.fileId !== video.id) return item
                              const next = { ...item }
                              if (event.currentTarget.value)
                                next.lastChapter = Number(event.currentTarget.value)
                              else delete next.lastChapter
                              return next
                            }),
                          )
                        }}
                      />
                    </div>
                  )}
                  {video.subtitles.map((subtitle) => {
                    const subtitleValue = subtitles.find((item) => item.fileId === subtitle.id) ?? {
                      fileId: subtitle.id,
                      language: subtitle.language,
                      forced: subtitle.forced,
                      sdh: subtitle.sdh,
                    }
                    const subtitleError = (field: string) =>
                      issueMessage(
                        translate,
                        planned?.errors.find(
                          (issue) => issue.file === subtitle.path && issue.field === field,
                        ),
                      )
                    return (
                      <div
                        key={subtitle.id}
                        className="grid gap-3 border-l-2 border-line pl-4 md:grid-cols-[minmax(0,1fr)_220px_auto_auto] md:items-end"
                      >
                        <TextField
                          label={t('assign.subtitles.file')}
                          value={subtitle.path}
                          readOnly
                        />
                        <Select
                          label={t('assign.subtitles.language')}
                          value={subtitleValue.language}
                          {...(subtitleError('language')
                            ? { error: subtitleError('language') }
                            : {})}
                          onChange={(event) => {
                            updateSubtitle(subtitle.id, { language: event.currentTarget.value })
                          }}
                        >
                          {languages.map(([code, label]) => (
                            <option key={code} value={code}>
                              {label}
                            </option>
                          ))}
                        </Select>
                        <Checkbox
                          label={t('assign.subtitles.forced')}
                          checked={subtitleValue.forced ?? false}
                          onChange={(forced) => {
                            updateSubtitle(subtitle.id, { forced })
                          }}
                        />
                        <Checkbox
                          label={t('assign.subtitles.sdh')}
                          checked={subtitleValue.sdh ?? false}
                          onChange={(sdh) => {
                            updateSubtitle(subtitle.id, { sdh })
                          }}
                        />
                      </div>
                    )
                  })}
                </article>
              )
            })}
          </section>
        </section>
        <EditPreview title={title} plan={planned} onSave={save} />
      </div>
      {openMatch && (
        <Dialog
          title={t('assign.titlePicker.dialogTitle')}
          subtitle={t('assign.titlePicker.dialogSubtitle')}
          onClose={() => {
            setOpenMatch(false)
          }}
          size="lg"
          actions={
            <Button
              onClick={() => {
                setOpenMatch(false)
              }}
            >
              {t('common.cancel')}
            </Button>
          }
        >
          <TextField
            label={t('assign.titlePicker.search')}
            value={term}
            onChange={(event) => {
              setTerm(event.currentTarget.value)
            }}
          />
          <TitleHitOptions
            key={term}
            library={libraryHits}
            tmdb={tmdbHits}
            selected={selectedHit}
            onPick={chooseHit}
          />
          <div className="flex flex-col gap-3 border-t border-line pt-4">
            <TextField
              label={t('assign.titlePicker.freeName')}
              value={freeTitle}
              onChange={(event) => {
                setFreeTitle(event.currentTarget.value)
              }}
            />
            <TextField
              label={t('assign.titlePicker.freeYear')}
              type="number"
              value={freeYear}
              onChange={(event) => {
                setFreeYear(event.currentTarget.value)
              }}
            />
            <Button variant="primary" disabled={!freeTitle.trim()} onClick={useFree}>
              {t('assign.titlePicker.useFree')}
            </Button>
          </div>
        </Dialog>
      )}
    </LibraryLayout>
  )
}

function EditPreview({
  title,
  plan,
  onSave,
}: {
  title: LibraryTitle
  plan: EditPlan | null
  onSave: () => void
}) {
  const { t } = useTranslation()
  const translate = t as unknown as Translate
  if (!plan)
    return (
      <section className="rounded-lg border border-line bg-surface p-5">
        <h2 className="m-0 text-heading font-display">{t('library.previewTitle')}</h2>
        <p className="text-body text-ink-2">{t('library.previewEmpty')}</p>
      </section>
    )
  const fileFolder = title.videos[0]?.path.split('/').slice(0, -1).join('/') ?? ''
  const oldFolder =
    title.kind === 'series' ? fileFolder.replace(/\/(?:Season \d+|Specials)$/, '') : fileFolder
  return (
    <aside className="flex flex-col gap-4 rounded-lg border border-line bg-surface p-5">
      <h2 className="m-0 text-heading font-display">{t('library.previewTitle')}</h2>
      {plan.folder !== oldFolder && (
        <PreviewChange before={oldFolder} after={plan.folder} label={t('library.rename')} />
      )}
      {plan.videos.map((video) =>
        video.changed ? (
          <PreviewChange
            key={video.fileId}
            before={video.file}
            after={video.finalPath ?? t('library.pendingPath')}
            label={t('library.rename')}
          />
        ) : (
          <p key={video.fileId} className="m-0 rounded-md bg-bg p-3 text-body-sm text-ink-2">
            {t('library.noChanges')}: <span className="font-mono-file">{video.file}</span>
          </p>
        ),
      )}
      {plan.subtitles.map((subtitle) =>
        subtitle.changed ? (
          <PreviewChange
            key={`s-${String(subtitle.fileId)}`}
            before={subtitle.file}
            after={subtitle.finalPath ?? t('library.pendingPath')}
            label={t('library.rename')}
          />
        ) : (
          <p
            key={`s-${String(subtitle.fileId)}`}
            className="m-0 rounded-md bg-bg p-3 text-body-sm text-ink-2"
          >
            {t('library.noChanges')}: <span className="font-mono-file">{subtitle.file}</span>
          </p>
        ),
      )}
      {plan.errors.map((issue) => (
        <p
          key={`${issue.file ?? 'title'}-${issue.field}-${issue.code}`}
          className="m-0 text-body-sm text-danger"
        >
          {issueMessage(translate, issue)}
        </p>
      ))}
      {plan.warnings.map((issue) => (
        <p
          key={`w-${issue.file ?? 'title'}-${issue.field}-${issue.code}`}
          className="m-0 text-body-sm text-warning-ink"
        >
          {issueMessage(translate, issue)}
        </p>
      ))}
      <Button variant="primary" disabled={!plan.valid} onClick={onSave}>
        {t('library.save')}
      </Button>
    </aside>
  )
}

function PreviewChange({ before, after, label }: { before: string; after: string; label: string }) {
  return (
    <div className="flex flex-col gap-1 rounded-md bg-bg p-3">
      <span className="text-caption font-bold uppercase text-success-ink">{label}</span>
      <span className="break-all font-mono-file text-caption text-ink-3 line-through">
        {before}
      </span>
      <span className="break-all font-mono-file text-caption text-ink-1">{after}</span>
    </div>
  )
}
