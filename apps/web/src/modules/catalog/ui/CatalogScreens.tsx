import { useMutation, useQueries } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '../application/ports'
import {
  useCommitTitleEdit,
  useLibraryCollection,
  useLibraryTitle,
  useLibraryTitles,
} from '../application/queries'
import type { LibraryTitle, LibraryTitleSummary } from '../domain/types'
import { useTitleSearch } from '@/modules/metadata/application/queries'
import { useMetadataPorts } from '@/modules/metadata/application/ports'
import { useJobs } from '@/modules/ingestion/application/queries'
import { Brand } from '@/shared/ui/Brand'
import {
  Banner,
  Button,
  Chip,
  ChipGroup,
  ConfirmDialog,
  EmptyState,
  IconButton,
  LinkButton,
  Menu,
  PageHeader,
  Poster,
  PosterRow,
  SearchField,
  SegmentedControl,
  StatusChip,
  Tabs,
  TextField,
} from '@/shared/ui'
import {
  ArrowRightIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  EditIcon,
  MoreIcon,
  PauseIcon,
  PlayIcon,
  RefreshIcon,
  SearchIcon,
  UploadIcon,
} from '@/shared/ui/icons'
import { formatDate, formatDuration } from '@/shared/i18n/format'
import { useLanguage } from '@/shared/i18n/hooks'

type Kind = 'movie' | 'series'
type Sort = 'added' | 'title' | 'year'

export function normalizeSearch(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase()
    .trim()
}

export function matchesSearch(title: LibraryTitleSummary, query: string): boolean {
  const words = normalizeSearch(query).split(/\s+/).filter(Boolean)
  const haystack = normalizeSearch(
    [title.title, title.originalTitle, title.discTitle].filter(Boolean).join(' '),
  )
  return words.every((word) => haystack.includes(word))
}

export function sortTitles(
  titles: readonly LibraryTitleSummary[],
  sort: Sort,
): LibraryTitleSummary[] {
  return [...titles].sort((a, b) => {
    if (sort === 'added') return Date.parse(b.addedAt) - Date.parse(a.addedAt)
    if (sort === 'year') return b.year - a.year || a.title.localeCompare(b.title)
    return a.title.localeCompare(b.title, undefined, { sensitivity: 'base' })
  })
}

function kindLabel(kind: Kind, t: TFunction) {
  return t(kind === 'movie' ? 'library.movie' : 'library.series')
}

function useReducedMotion() {
  const [reduced, setReduced] = useState(false)
  useEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => {
      setReduced(media.matches)
    }
    update()
    media.addEventListener('change', update)
    return () => {
      media.removeEventListener('change', update)
    }
  }, [])
  return reduced
}

function Header() {
  const { t } = useTranslation()
  const language = useLanguage()
  const jobs = useJobs()
  const active = (jobs.data ?? []).filter((job) =>
    ['queued', 'uploading', 'extracting', 'filtering', 'needs_password'].includes(job.status),
  ).length
  const links = [
    ['/peliculas', t('library.movies')],
    ['/series', t('library.seriesPlural')],
  ]
  return (
    <header className="sticky top-0 z-30 border-b border-line bg-bg/95 px-[var(--page-x,16px)] backdrop-blur">
      <div className="mx-auto flex min-h-16 max-w-screen-2xl items-center gap-4">
        <Brand />
        <nav aria-label={t('library.mainNavigation')} className="hidden items-center gap-5 md:flex">
          <a href="/" className="text-body-sm font-semibold no-underline">
            {t('library.home')}
          </a>
          {links.map(([href, label]) => (
            <a key={href} href={href} className="text-body-sm font-semibold no-underline">
              {label}
            </a>
          ))}
        </nav>
        <form
          action="/buscar"
          method="get"
          className="ml-auto hidden min-w-48 max-w-72 flex-1 md:block"
        >
          <SearchField
            label={t('library.search')}
            name="q"
            placeholder={t('library.searchPlaceholder')}
            shortcut
          />
        </form>
        <a
          href="/buscar"
          aria-label={t('library.search')}
          className="inline-flex size-control-sm items-center justify-center rounded-full md:hidden"
        >
          <SearchIcon />
        </a>
        <button
          type="button"
          className="relative inline-flex min-h-control-sm items-center gap-2 rounded-pill border border-control px-3 text-body-sm font-bold"
        >
          <UploadIcon />
          <span className="hidden lg:inline">{t('library.upload')}</span>
          {active > 0 && (
            <span
              aria-label={t('library.uploadsActive', { count: active })}
              className="size-2 rounded-full bg-accent"
            />
          )}
        </button>
        <a
          href="/ajustes/general"
          aria-label={t('library.settings')}
          className="inline-flex size-control-sm items-center justify-center rounded-full text-ink-2"
        >
          ⚙
        </a>
        <span className="sr-only">{language}</span>
      </div>
    </header>
  )
}

function MobileNav() {
  const { t } = useTranslation()
  const links = [
    ['/', '⌂', t('library.home')],
    ['/peliculas', '▣', t('library.movies')],
    ['/series', '▤', t('library.seriesPlural')],
    ['/no-asignados', '□', t('library.unassigned')],
    ['/ajustes/general', '⚙', t('library.settings')],
  ]
  return (
    <nav
      aria-label={t('library.mobileNavigation')}
      className="fixed inset-x-0 bottom-0 z-30 grid grid-cols-5 border-t border-line bg-surface px-1 pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      {links.map(([href, icon, label]) => (
        <a
          key={href}
          href={href}
          className="flex min-h-16 flex-col items-center justify-center gap-0.5 text-caption text-ink-2 no-underline"
        >
          <span aria-hidden="true" className="text-lg">
            {icon}
          </span>
          {label}
        </a>
      ))}
    </nav>
  )
}

function LibraryLayout({ children }: { children: ReactNode }) {
  return (
    <>
      <Header />
      <main className="mx-auto min-h-app max-w-screen-2xl page-padding pb-24 md:pb-10">
        {children}
      </main>
      <MobileNav />
    </>
  )
}

function LoadingGrid() {
  return (
    <div
      aria-hidden="true"
      className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6"
    >
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="aspect-[2/3] animate-pulse rounded-md bg-surface" />
      ))}
    </div>
  )
}

function PosterFor({ title, href }: { title: LibraryTitleSummary; href?: string }) {
  const { t } = useTranslation()
  const chips = [
    title.missingEpisodes !== undefined ? (
      <StatusChip key="missing" tone="warning" solid>
        {t('library.missingCount', { count: title.missingEpisodes })}
      </StatusChip>
    ) : null,
    !title.tmdbId ? (
      <StatusChip key="free" tone="warning" solid>
        {t('library.unlinked')}
      </StatusChip>
    ) : null,
  ].filter(Boolean) as ReactNode[]
  const meta = [title.year || undefined, kindLabel(title.kind, t)].filter(Boolean).join(' · ')
  const posterProps = {
    title: title.title,
    meta,
    chips,
    accessibleChips: chips.map(() => ''),
    seed: String(title.id),
    ...(title.poster ? { image: title.poster } : {}),
    href: href ?? `/${title.kind === 'movie' ? 'peliculas' : 'series'}/${String(title.id)}`,
  }
  return <Poster {...posterProps} />
}

function HeroCarousel({ titles }: { titles: LibraryTitleSummary[] }) {
  const { t } = useTranslation()
  const ports = useCatalogPorts()
  const reduced = useReducedMotion()
  const [index, setIndex] = useState(0)
  const [paused, setPaused] = useState(false)
  const details = useQueries({
    queries: titles.map((title) => ({
      queryKey: ['library', 'titles', title.id],
      queryFn: () => ports.libraryTitle(title.id),
    })),
  })
  const current = titles[index]
  const detail = details[index]?.data
  useEffect(() => {
    if (reduced || paused || titles.length < 2) return undefined
    const timer = window.setInterval(() => {
      setIndex((value) => (value + 1) % titles.length)
    }, 8000)
    return () => {
      window.clearInterval(timer)
    }
  }, [paused, reduced, titles.length])
  if (!current) return null
  const move = (delta: number) => {
    setIndex((value) => (value + delta + titles.length) % titles.length)
  }
  return (
    <section
      aria-label={t('library.heroLabel')}
      className="relative -mx-[var(--page-x,16px)] min-h-[440px] overflow-hidden"
      onMouseEnter={() => {
        setPaused(true)
      }}
      onMouseLeave={() => {
        setPaused(false)
      }}
      onFocus={() => {
        setPaused(true)
      }}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setPaused(false)
      }}
    >
      {current.backdrop && (
        <img src={current.backdrop} alt="" className="absolute inset-0 size-full object-cover" />
      )}
      <div className="absolute inset-0 bg-scrim" />
      <div className="relative flex min-h-[440px] max-w-3xl flex-col justify-end gap-4 px-[var(--page-x,16px)] py-12">
        <span className="text-label font-bold uppercase tracking-label text-accent">
          {t('library.featured')}
        </span>
        <h2 className="m-0 max-w-3xl text-display-hero font-display uppercase">
          {detail?.logo ? (
            <img
              src={detail.logo}
              alt={detail.title}
              className="max-h-24 max-w-[min(70vw,480px)] object-contain object-left"
            />
          ) : (
            (detail?.title ?? current.title)
          )}
        </h2>
        <p className="m-0 flex flex-wrap gap-3 text-body-sm text-ink-2">
          {detail?.year ?? current.year}
          {detail?.genres.length ? ` · ${detail.genres.join(' · ')}` : ''}
        </p>
        {detail?.overview && (
          <p className="m-0 max-w-2xl text-body-lg text-ink-1">{detail.overview}</p>
        )}
        <div className="flex flex-wrap gap-3">
          <LinkButton
            href={`/${current.kind === 'movie' ? 'peliculas' : 'series'}/${String(current.id)}`}
            variant="primary"
            icon={<ArrowRightIcon />}
          >
            {t('library.details')}
          </LinkButton>
          <LinkButton
            href={`/${current.kind === 'movie' ? 'peliculas' : 'series'}/${String(current.id)}/editar`}
            variant="secondary"
            icon={<EditIcon />}
          >
            {t('library.edit')}
          </LinkButton>
        </div>
        <div
          className="flex items-center gap-2"
          role="group"
          aria-label={t('library.heroIndicators')}
        >
          <IconButton
            label={t('common.previous')}
            variant="overlay"
            onClick={() => {
              move(-1)
            }}
          >
            <ChevronLeftIcon />
          </IconButton>
          {titles.map((title, i) => (
            <button
              key={title.id}
              type="button"
              aria-label={t('library.heroGoTo', { count: i + 1 })}
              aria-current={i === index ? 'true' : undefined}
              onClick={() => {
                setIndex(i)
              }}
              className={`size-11 rounded-full border ${i === index ? 'border-accent bg-accent' : 'border-control bg-surface/70'}`}
            />
          ))}
          <IconButton
            label={t('common.next')}
            variant="overlay"
            onClick={() => {
              move(1)
            }}
          >
            <ChevronRightIcon />
          </IconButton>
          <IconButton
            label={paused ? t('library.resumeHero') : t('library.pauseHero')}
            variant="overlay"
            onClick={() => {
              setPaused((value) => !value)
            }}
          >
            {paused ? <PlayIcon /> : <PauseIcon />}
          </IconButton>
        </div>
      </div>
    </section>
  )
}

function HomeScreen() {
  const { t } = useTranslation()
  const query = useLibraryTitles()
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
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )
  const titles = query.data.titles
  if (!titles.length)
    return (
      <LibraryLayout>
        <div className="flex min-h-[70vh] flex-col justify-center gap-5">
          <h1 className="sr-only">{t('library.home')}</h1>
          <EmptyState
            title={t('library.emptyTitle')}
            body={t('library.emptyBody')}
            action={
              <Button variant="primary" icon={<UploadIcon />}>
                {t('library.upload')}
              </Button>
            }
          />
        </div>
      </LibraryLayout>
    )
  const recent = sortTitles(titles, 'added')
  const hero = recent.filter((title) => title.backdrop).slice(0, 5)
  const movies = titles.filter((title) => title.kind === 'movie')
  const series = titles.filter((title) => title.kind === 'series')
  const genres = [...new Set(titles.flatMap((title) => title.genres ?? []))]
  const collections = [
    ...new Map(
      titles
        .filter((title) => title.collection)
        .map((title) => [title.collection?.id, title.collection]),
    ).values(),
  ].filter(Boolean)
  return (
    <LibraryLayout>
      <h1 className="sr-only">{t('library.home')}</h1>
      {hero.length > 0 && <HeroCarousel titles={hero} />}
      <div className="flex flex-col gap-[var(--row-gap,36px)] py-8">
        <PendingStrip titles={titles} />
        <PosterRow
          title={t('library.recent')}
          seeAll={
            <a href="/peliculas" className="text-body-sm">
              {t('library.seeAll')}
            </a>
          }
        >
          {recent.map((title) => (
            <PosterFor key={title.id} title={title} />
          ))}
        </PosterRow>
        <PosterRow
          title={t('library.movies')}
          seeAll={
            <a href="/peliculas" className="text-body-sm">
              {t('library.seeAll')}
            </a>
          }
        >
          {movies.map((title) => (
            <PosterFor key={title.id} title={title} />
          ))}
        </PosterRow>
        <PosterRow
          title={t('library.seriesPlural')}
          seeAll={
            <a href="/series" className="text-body-sm">
              {t('library.seeAll')}
            </a>
          }
        >
          {series.map((title) => (
            <PosterFor key={title.id} title={title} />
          ))}
        </PosterRow>
        {genres.map((genre) => (
          <PosterRow key={genre} title={genre}>
            {titles
              .filter((title) => title.genres?.includes(genre))
              .map((title) => (
                <PosterFor key={title.id} title={title} />
              ))}
          </PosterRow>
        ))}
        {collections.length > 0 && (
          <PosterRow title={t('library.collections')}>
            {collections.map((collection) => {
              const member = titles.find((title) => title.collection?.id === collection?.id)
              return (
                member && (
                  <PosterFor
                    key={collection?.id}
                    title={member}
                    href={`/colecciones/${String(collection?.id)}`}
                  />
                )
              )
            })}
          </PosterRow>
        )}
      </div>
    </LibraryLayout>
  )
}

function PendingStrip({ titles }: { titles: LibraryTitleSummary[] }) {
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

function BrowseScreen({
  kind,
  sort = 'added',
  genre,
  incomplete = false,
}: {
  kind: Kind
  sort?: string | undefined
  genre?: string | undefined
  incomplete?: boolean | undefined
}) {
  const { t } = useTranslation()
  const query = useLibraryTitles()
  const [selectedSort, setSelectedSort] = useState<Sort>(
    sort === 'title' || sort === 'year' ? sort : 'added',
  )
  const [selectedGenre, setSelectedGenre] = useState(genre ?? '')
  const [onlyIncomplete, setOnlyIncomplete] = useState(incomplete)
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
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )
  const all = query.data.titles.filter((title) => title.kind === kind)
  const genres = [...new Set(all.flatMap((title) => title.genres ?? []))]
  const filtered = sortTitles(
    all.filter(
      (title) =>
        (!selectedGenre || title.genres?.includes(selectedGenre)) &&
        (!onlyIncomplete || (title.missingEpisodes ?? 0) > 0),
    ),
    selectedSort,
  )
  return (
    <LibraryLayout>
      <PageHeader
        title={kindLabel(kind, t)}
        subtitle={t('library.titleCount', { count: filtered.length })}
      />
      <div className="flex flex-wrap items-center justify-between gap-4 py-6">
        <SegmentedControl
          label={t('library.sort')}
          options={[
            { value: 'added', label: t('library.sortAdded') },
            { value: 'title', label: t('library.sortTitle') },
            { value: 'year', label: t('library.sortYear') },
          ]}
          value={selectedSort}
          onChange={(value) => {
            setSelectedSort(value)
          }}
        />
        <ChipGroup
          legend={t('library.genre')}
          name="genre"
          options={[
            { value: '', label: t('library.allGenres') },
            ...genres.map((value) => ({ value, label: value })),
          ]}
          value={selectedGenre}
          onChange={setSelectedGenre}
        />
      </div>
      {kind === 'series' && (
        <label className="mb-6 flex min-h-control-sm items-center gap-3 text-body">
          <input
            type="checkbox"
            checked={onlyIncomplete}
            onChange={(event) => {
              setOnlyIncomplete(event.currentTarget.checked)
            }}
            className="size-5 accent-accent"
          />
          {t('library.onlyIncomplete')}
        </label>
      )}
      {filtered.length ? (
        <div className="grid grid-cols-2 gap-x-4 gap-y-8 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6">
          {filtered.map((title) => (
            <PosterFor key={title.id} title={title} />
          ))}
        </div>
      ) : (
        <EmptyState title={t('library.emptyFilterTitle')} body={t('library.emptyFilterBody')} />
      )}
    </LibraryLayout>
  )
}

function TitleHeader({
  title,
  onRefresh,
  refreshing,
}: {
  title: LibraryTitle
  onRefresh: () => void
  refreshing: boolean
}) {
  const { t } = useTranslation()
  return (
    <section className="relative -mx-[var(--page-x,16px)] overflow-hidden border-b border-line">
      <div className="relative min-h-[360px] p-6 pt-12 md:p-12">
        {title.backdrop && (
          <img src={title.backdrop} alt="" className="absolute inset-0 size-full object-cover" />
        )}
        <div className="absolute inset-0 bg-scrim" />
        <div className="relative flex max-w-4xl flex-col gap-4 md:flex-row md:items-end">
          <div className="hidden w-40 shrink-0 md:block">
            {title.poster ? (
              <img
                src={title.poster}
                alt=""
                className="aspect-[2/3] w-full rounded-md object-cover"
              />
            ) : (
              <div className="aspect-[2/3] rounded-md bg-surface-missing" />
            )}
          </div>
          <div className="flex flex-col gap-3">
            <span className="text-label font-bold uppercase tracking-label text-accent">
              {kindLabel(title.kind, t)}
            </span>
            <h1 className="m-0 text-display font-display">{title.title}</h1>
            <p className="m-0 text-body-sm text-ink-2">
              {title.year || t('library.unknownYear')}
              {title.originalTitle ? ` · ${title.originalTitle}` : ''}
            </p>
            {title.genres.length > 0 && (
              <div className="flex flex-wrap gap-2">
                {title.genres.map((genre) => (
                  <Chip key={genre}>{genre}</Chip>
                ))}
              </div>
            )}
            <p className="m-0 max-w-3xl text-body-lg">
              {title.overview || t('library.noSynopsis')}
            </p>
            <div className="flex flex-wrap gap-3">
              <LinkButton
                href={`/peliculas/${String(title.id)}/editar`}
                variant="primary"
                icon={<EditIcon />}
              >
                {title.tmdbId ? t('library.edit') : t('library.link')}
              </LinkButton>
              <Button
                variant="secondary"
                loading={refreshing}
                icon={<RefreshIcon />}
                onClick={onRefresh}
              >
                {t('library.refreshData')}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

function VersionCard({
  titleId,
  video,
  kind,
}: {
  titleId: number
  video: LibraryTitle['videos'][number]
  kind: Kind
}) {
  const { t } = useTranslation()
  const ports = useCatalogPorts()
  const [technical, setTechnical] = useState(false)
  const [confirm, setConfirm] = useState<'move' | 'trash' | null>(null)
  const language = useLanguage()
  const move = useMutation({
    mutationFn: () => ports.moveTitleToUnassigned(titleId, { kind: 'file', fileId: video.id }),
    onSuccess: () => {
      setConfirm(null)
    },
  })
  const trash = useMutation({
    mutationFn: () => ports.trashTitleScope(titleId, { kind: 'file', fileId: video.id }),
    onSuccess: () => {
      setConfirm(null)
    },
  })
  const technicalQuery = useQueries({
    queries: [
      {
        queryKey: ['library', 'technical', titleId, video.id],
        queryFn: () => ports.fileTechnical(titleId, video.id),
        enabled: technical,
      },
    ],
  })[0]
  return (
    <article className="flex flex-col gap-3 rounded-lg border border-line bg-surface p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="m-0 text-subheading font-display">{video.path.split('/').at(-1)}</h3>
          <p className="m-0 mt-1 font-mono-file text-mono text-ink-2">{video.path}</p>
          <p className="m-0 mt-1 text-body-sm text-ink-2">
            {video.part ? t('library.part', { count: video.part }) : ''}
            {video.version ? ` · ${video.version}` : ''} ·{' '}
            {t('library.bytes', { count: video.size })}
          </p>
        </div>
        <Menu
          label={t('library.fileActions')}
          items={[
            {
              label: t('library.download'),
              onSelect: () => {
                window.location.assign(ports.downloadFileUrl(titleId, video.id))
              },
            },
            {
              label: t('library.moveUnassigned'),
              onSelect: () => {
                setConfirm('move')
              },
            },
            {
              label: t('library.trash'),
              tone: 'danger',
              onSelect: () => {
                setConfirm('trash')
              },
            },
          ]}
        >
          <MoreIcon />
        </Menu>
      </div>
      {video.subtitles.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {video.subtitles.map((subtitle) => (
            <StatusChip key={subtitle.id} tone="success">
              {subtitle.language}
              {subtitle.forced ? ` · ${t('library.forced')}` : ''}
              {subtitle.sdh ? ` · ${t('library.sdh')}` : ''}
            </StatusChip>
          ))}
        </div>
      )}
      <Button
        variant="secondary"
        size="sm"
        onClick={() => {
          setTechnical((value) => !value)
        }}
      >
        {t('library.technicalInfo')}
      </Button>
      {technical && technicalQuery.data && (
        <dl className="grid auto-fit grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 border-t border-line pt-3 text-body-sm">
          <div>
            <dt className="text-ink-3">{t('library.duration')}</dt>
            <dd className="m-0">{formatDuration(technicalQuery.data.duration, language, t)}</dd>
          </div>
          <div>
            <dt className="text-ink-3">{t('library.resolution')}</dt>
            <dd className="m-0">
              {technicalQuery.data.video.width} × {technicalQuery.data.video.height}
            </dd>
          </div>
          <div>
            <dt className="text-ink-3">{t('library.codec')}</dt>
            <dd className="m-0">{technicalQuery.data.video.codec}</dd>
          </div>
          <div>
            <dt className="text-ink-3">{t('library.download')}</dt>
            <dd className="m-0">
              <a href={ports.downloadFileUrl(titleId, video.id)}>{t('library.download')}</a>
            </dd>
          </div>
        </dl>
      )}
      {kind === 'movie' && (
        <a href={ports.downloadFileUrl(titleId, video.id)} className="text-body-sm">
          {t('library.download')}
        </a>
      )}
      {confirm === 'move' && (
        <ConfirmDialog
          title={t('library.confirmFileMoveTitle')}
          confirmLabel={t('library.moveUnassigned')}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            move.mutate()
          }}
          busy={move.isPending}
        >
          {t('library.confirmFileMoveBody')}
        </ConfirmDialog>
      )}
      {confirm === 'trash' && (
        <ConfirmDialog
          tone="danger"
          title={t('library.confirmFileTrashTitle')}
          confirmLabel={t('library.trash')}
          onCancel={() => {
            setConfirm(null)
          }}
          onConfirm={() => {
            trash.mutate()
          }}
          busy={trash.isPending}
        >
          {t('library.confirmFileTrashBody')}
        </ConfirmDialog>
      )}
    </article>
  )
}

function EpisodeList({
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

function DetailScreen({ id, kind }: { id: number; kind: Kind }) {
  const { t } = useTranslation()
  const query = useLibraryTitle(id)
  const metadata = useMetadataPorts()
  const ports = useCatalogPorts()
  const [confirm, setConfirm] = useState<'unassigned' | 'trash' | null>(null)
  const [season, setSeason] = useState('0')
  const refresh = useMutation({
    mutationFn: () => metadata.refreshTitleMetadata(id),
    onSuccess: () => undefined,
  })
  const move = useMutation({
    mutationFn: () => ports.moveTitleToUnassigned(id, { kind: 'title' }),
    onSuccess: () => {
      setConfirm(null)
    },
  })
  const trash = useMutation({
    mutationFn: () => ports.trashTitleScope(id, { kind: 'title' }),
    onSuccess: () => {
      setConfirm(null)
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
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )
  const title = query.data
  const selectedSeason =
    title.seasons.find((value) => value.number === Number(season)) ?? title.seasons[0]
  return (
    <LibraryLayout>
      <TitleHeader
        title={title}
        onRefresh={() => {
          refresh.mutate()
        }}
        refreshing={refresh.isPending}
      />
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
                label:
                  value.number === 0
                    ? t('library.specials')
                    : t('library.season', { number: value.number }),
              }))}
              value={selectedSeason ? String(selectedSeason.number) : season}
              onChange={setSeason}
            />
            {selectedSeason && <EpisodeList title={title} season={selectedSeason} />}
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

function CollectionInline({ collection }: { collection: NonNullable<LibraryTitle['collection']> }) {
  const { t } = useTranslation()
  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-heading font-display">{t('library.collection')}</h2>
        <a href={`/colecciones/${String(collection.id)}`}>{t('library.viewCollection')}</a>
      </div>
      <div className="mt-3 flex gap-4 overflow-x-auto">
        {collection.movies.map((movie) => (
          <Poster
            key={movie.id}
            title={movie.title}
            meta={String(movie.year)}
            dimmed={!movie.inLibrary}
            {...(movie.poster ? { image: movie.poster } : {})}
            {...(movie.libraryTitleId
              ? { href: `/peliculas/${String(movie.libraryTitleId)}` }
              : {})}
          />
        ))}
      </div>
    </section>
  )
}

function CollectionScreen({ id }: { id: number }) {
  const { t } = useTranslation()
  const query = useLibraryCollection(id)
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
          <EmptyState title={t('errors.loadTitle')} body={t('errors.loadBody')} />
        </div>
      </LibraryLayout>
    )
  const collection = query.data
  return (
    <LibraryLayout>
      <PageHeader title={collection.name} subtitle={collection.overview} />
      <div className="grid grid-cols-2 gap-x-4 gap-y-8 py-8 sm:grid-cols-3 lg:grid-cols-5">
        {collection.movies.map((movie) => (
          <Poster
            key={movie.id}
            title={movie.title}
            meta={`${String(movie.year)} · ${movie.originalTitle}`}
            dimmed={!movie.inLibrary}
            {...(movie.poster ? { image: movie.poster } : {})}
            {...(movie.libraryTitleId
              ? { href: `/peliculas/${String(movie.libraryTitleId)}` }
              : {})}
          />
        ))}
      </div>
    </LibraryLayout>
  )
}

function SearchScreen({ queryText }: { queryText: string }) {
  const { t } = useTranslation()
  const query = useLibraryTitles()
  const [value, setValue] = useState(queryText)
  const results = query.data?.titles.filter((title) => matchesSearch(title, value)) ?? []
  return (
    <LibraryLayout>
      <PageHeader title={t('library.search')} />
      <div className="py-6">
        <SearchField
          label={t('library.search')}
          value={value}
          onChange={(event) => {
            setValue(event.currentTarget.value)
          }}
          onClear={() => {
            setValue('')
          }}
          placeholder={t('library.searchPlaceholder')}
        />
      </div>
      {query.isPending ? (
        <LoadingGrid />
      ) : results.length ? (
        <div className="grid grid-cols-2 gap-x-4 gap-y-8 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6">
          {results.map((title) => (
            <PosterFor key={title.id} title={title} />
          ))}
        </div>
      ) : (
        <EmptyState title={t('library.noResults')} body={t('library.noResultsBody')} />
      )}
    </LibraryLayout>
  )
}

function EditScreen({ id }: { id: number }) {
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
    if (!planned) return
    commit.mutate({ ...request(), revision: planned.revision })
  }
  return (
    <LibraryLayout>
      <PageHeader
        back={<a href={`/peliculas/${String(id)}`}>{t('common.previous')}</a>}
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
          <Button
            variant="secondary"
            onClick={() => {
              makePlan()
            }}
          >
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
              <Button
                variant="primary"
                disabled={!planned.valid}
                onClick={() => {
                  save()
                }}
              >
                {t('library.save')}
              </Button>
            </>
          ) : (
            <p className="text-body text-ink-2">{t('library.previewEmpty')}</p>
          )}
        </section>
      </div>
      {commit.isError && <Banner tone="danger">{t('library.planChanged')}</Banner>}
    </LibraryLayout>
  )
}

export function CatalogScreen({
  screen,
  id,
  search,
}: {
  screen: string
  id?: number | undefined
  search?:
    | {
        orden?: string | undefined
        genero?: string | undefined
        incompletas?: string | undefined
        q?: string | undefined
      }
    | undefined
}) {
  if (screen === 'home') return <HomeScreen />
  if (screen === 'movies')
    return <BrowseScreen kind="movie" sort={search?.orden} genre={search?.genero} />
  if (screen === 'series')
    return (
      <BrowseScreen
        kind="series"
        sort={search?.orden}
        genre={search?.genero}
        incomplete={search?.incompletas === '1'}
      />
    )
  if (screen === 'movieDetail' && id) return <DetailScreen id={id} kind="movie" />
  if (screen === 'seriesDetail' && id) return <DetailScreen id={id} kind="series" />
  if (screen === 'collection' && id) return <CollectionScreen id={id} />
  if (screen === 'search') return <SearchScreen queryText={search?.q ?? ''} />
  if (screen === 'edit' && id) return <EditScreen id={id} />
  return (
    <LibraryLayout>
      <EmptyState title="" body="" />
    </LibraryLayout>
  )
}
