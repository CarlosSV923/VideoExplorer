import { useMemo, useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { useLibraryTitles, libraryTitlesKey } from '../application/queries'
import { useCatalogPorts } from '../application/ports'
import { toAppError } from '@/shared/kernel/errors'
import { type Kind, sortTitles, type Sort } from '../application/search'
import { LibraryLayout } from '@/app/LibraryLayout'
import {
  Banner,
  Button,
  ChipGroup,
  ConfirmDialog,
  EmptyState,
  HelpBar,
  PageHeader,
  SegmentedControl,
} from '@/shared/ui'
import { useAction, useGamepadConnected } from '@/shared/input'
import { LoadingGrid, PosterFor } from './CatalogShared'

export function BrowseScreen({
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
  const router = useRouter({ warn: false }) as ReturnType<typeof useRouter> | undefined
  const query = useLibraryTitles()
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  const gamepad = useGamepadConnected()
  const [selectionMode, setSelectionMode] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState(0)
  const [result, setResult] = useState<{
    ok: number
    failed: { id: number; title: string; reason: string }[]
  } | null>(null)
  const [selectedSort, setSelectedSort] = useState<Sort>(
    sort === 'title' || sort === 'year' ? sort : 'added',
  )
  const [selectedGenre, setSelectedGenre] = useState(genre ?? '')
  const [onlyIncomplete, setOnlyIncomplete] = useState(incomplete)
  const selectedTitles = useMemo(
    () => query.data?.titles.filter((title) => selectedIds.has(title.id)) ?? [],
    [query.data, selectedIds],
  )
  const toggle = (id: number) => {
    setSelectedIds((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  const cancelSelection = () => {
    if (!busy) {
      setSelectionMode(false)
      setSelectedIds(new Set())
      setConfirm(false)
    }
  }
  useAction(
    'back',
    () => {
      if (selectionMode && !confirm) {
        cancelSelection()
        return true
      }
      return undefined
    },
    selectionMode,
  )
  useAction(
    'confirm',
    () => {
      if (!selectionMode || confirm || busy) return undefined
      const active = document.activeElement as HTMLElement | null
      const id = Number(active?.dataset.selectionId)
      if (id) {
        toggle(id)
        return true
      }
      return undefined
    },
    selectionMode,
  )
  useAction(
    'menu',
    () => {
      if (selectionMode && selectedIds.size && !busy) {
        setConfirm(true)
        return true
      }
      return undefined
    },
    selectionMode,
  )
  const sendToTrash = async () => {
    setBusy(true)
    setProgress(0)
    const failed: { id: number; title: string; reason: string }[] = []
    let ok = 0
    for (const [index, title] of selectedTitles.entries()) {
      setProgress(index + 1)
      try {
        await ports.trashTitleScope(title.id, { kind: 'title' })
        ok++
      } catch (error) {
        failed.push({
          id: title.id,
          title: title.title,
          reason: t(`library.selectionTrashError.${toAppError(error).kind}`),
        })
      }
    }
    await queryClient.invalidateQueries({ queryKey: libraryTitlesKey })
    setBusy(false)
    setConfirm(false)
    setResult({ ok, failed })
    if (!failed.length) {
      setSelectionMode(false)
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set<number>(failed.map((failure) => failure.id)))
    }
  }
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
        title={kind === 'movie' ? t('library.movies') : t('library.seriesPlural')}
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
            const search = { sort: value, genre: selectedGenre || undefined }
            if (kind === 'movie') void router?.navigate({ to: '/movies', search })
            else
              void router?.navigate({
                to: '/series',
                search: { ...search, incomplete: onlyIncomplete ? 1 : undefined },
              })
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
          onChange={(value) => {
            setSelectedGenre(value)
            const search = { sort: selectedSort, genre: value || undefined }
            if (kind === 'movie') void router?.navigate({ to: '/movies', search })
            else
              void router?.navigate({
                to: '/series',
                search: { ...search, incomplete: onlyIncomplete ? 1 : undefined },
              })
          }}
        />
        <Button
          size="sm"
          disabled={busy}
          onClick={() => {
            setSelectionMode((value) => !value)
            setSelectedIds(new Set())
            setResult(null)
          }}
        >
          {t(selectionMode ? 'library.selectionCancel' : 'library.selectionEnter')}
        </Button>
      </div>
      {kind === 'series' && (
        <label className="mb-6 flex min-h-control-sm items-center gap-3 text-body">
          <input
            type="checkbox"
            checked={onlyIncomplete}
            onChange={(event) => {
              const incomplete = event.currentTarget.checked
              setOnlyIncomplete(incomplete)
              void router?.navigate({
                to: '/series',
                search: {
                  sort: selectedSort,
                  genre: selectedGenre || undefined,
                  incomplete: incomplete ? 1 : undefined,
                },
              })
            }}
            className="size-5 accent-accent"
          />
          {t('library.onlyIncomplete')}
        </label>
      )}
      {result && (
        <div className="mb-5">
          <Banner
            tone={result.failed.length ? 'danger' : 'info'}
            live={result.failed.length ? 'assertive' : 'polite'}
            action={
              !result.failed.length ? (
                <Button
                  size="sm"
                  onClick={() => {
                    setResult(null)
                  }}
                >
                  {t('common.close')}
                </Button>
              ) : undefined
            }
          >
            {result.failed.length ? (
              <>
                {t('library.selectionFailed')}
                {result.failed.map((failure) => (
                  <div key={failure.title}>
                    {failure.title}: {failure.reason}
                  </div>
                ))}
              </>
            ) : (
              t('library.selectionTrashed', { count: result.ok })
            )}
          </Banner>
        </div>
      )}
      {filtered.length ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-x-4 gap-y-8 sm:grid-cols-[repeat(auto-fill,minmax(176px,1fr))]">
          {filtered.map((title) => (
            <PosterFor
              key={title.id}
              title={title}
              fluid
              selected={selectedIds.has(title.id)}
              {...(selectionMode
                ? {
                    selectionId: title.id,
                    onToggle: () => {
                      toggle(title.id)
                    },
                  }
                : {})}
            />
          ))}
        </div>
      ) : (
        <EmptyState title={t('library.emptyFilterTitle')} body={t('library.emptyFilterBody')} />
      )}
      {selectionMode && (
        <>
          <div className="fixed inset-x-0 bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-20 mx-auto max-w-screen-2xl border border-line bg-surface-2 p-4 shadow-xl md:bottom-4 md:rounded-xl">
            <div className="page-padding flex flex-wrap items-center justify-between gap-3">
              <div aria-live="polite" className="font-semibold">
                {t('library.selectionCount', { count: selectedIds.size })}
              </div>
              <div className="grid w-full grid-cols-2 gap-2 md:flex md:w-auto">
                <Button
                  size="sm"
                  className="w-full md:w-auto"
                  onClick={() => {
                    setSelectedIds((current) =>
                      filtered.every((title) => current.has(title.id))
                        ? new Set(
                            [...current].filter((id) => !filtered.some((title) => title.id === id)),
                          )
                        : new Set([...current, ...filtered.map((title) => title.id)]),
                    )
                  }}
                >
                  {t(
                    filtered.length > 0 && filtered.every((title) => selectedIds.has(title.id))
                      ? 'library.selectionDeselectAll'
                      : 'library.selectionSelectAll',
                  )}
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  className="w-full md:w-auto"
                  disabled={!selectedIds.size || busy}
                  onClick={() => {
                    setConfirm(true)
                    setProgress(0)
                  }}
                >
                  {busy
                    ? t('library.selectionProgress', {
                        current: progress,
                        total: selectedTitles.length,
                      })
                    : t('library.trash')}
                </Button>
                <Button
                  size="sm"
                  className="w-full md:w-auto"
                  onClick={cancelSelection}
                  disabled={busy}
                >
                  {t('library.selectionCancel')}
                </Button>
              </div>
            </div>
            {gamepad && (
              <HelpBar
                actions={[
                  { glyph: 'A', label: t('library.selectionMark') },
                  { glyph: 'B', label: t('library.selectionCancel') },
                  { glyph: 'X', label: t('library.trash') },
                ]}
              />
            )}
          </div>
          {confirm && (
            <ConfirmDialog
              tone="danger"
              busy={busy}
              title={t('library.selectionConfirmTitle', { count: selectedTitles.length })}
              confirmLabel={
                busy
                  ? t('library.selectionProgress', {
                      current: progress,
                      total: selectedTitles.length,
                    })
                  : t('library.trash')
              }
              onCancel={() => {
                if (!busy) setConfirm(false)
              }}
              onConfirm={() => void sendToTrash()}
            >
              <ul className="m-0 list-disc pl-6">
                {selectedTitles.slice(0, 5).map((title) => (
                  <li key={title.id}>{title.title}</li>
                ))}
                {selectedTitles.length > 5 && (
                  <li>{t('library.selectionMore', { count: selectedTitles.length - 5 })}</li>
                )}
              </ul>
              <p>{t('library.selectionRestoreNotice')}</p>
            </ConfirmDialog>
          )}
        </>
      )}
    </LibraryLayout>
  )
}
