import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useTitleSearch } from '@/modules/metadata/application/queries'
import { useMetadataPorts } from '@/modules/metadata/application/ports'
import type { TitleChoice, TitleHit } from '../domain/types'
import { titleChoiceFromHit } from '../application/assignment'
import { Button, Dialog, TextField } from '@/shared/ui'
import { kindLabel } from './CatalogLabels'

function sortHits(hits: readonly TitleHit[], preferred: 'movie' | 'series') {
  return [...hits].sort((left, right) => {
    const leftRank = left.kind === preferred ? 0 : 1
    const rightRank = right.kind === preferred ? 0 : 1
    return leftRank - rightRank
  })
}

export function TitlePicker({
  value,
  suggested,
  initialTerm = '',
  kind = 'movie',
  error,
  onChange,
}: {
  value?: TitleChoice
  suggested?: TitleHit
  initialTerm?: string
  kind?: 'movie' | 'series'
  error?: string | undefined
  onChange: (value: TitleChoice) => void
}) {
  const { t } = useTranslation()
  const metadata = useMetadataPorts()
  const [open, setOpen] = useState(false)
  const [selectedHit, setSelectedHit] = useState<TitleHit | undefined>(suggested)
  const [term, setTerm] = useState(initialTerm)
  const [freeTitle, setFreeTitle] = useState('')
  const [freeYear, setFreeYear] = useState('')
  const search = useTitleSearch(term)
  const selectedLabel = value?.source === 'free' ? value.title : undefined
  const displayTitle = selectedHit?.title ?? selectedLabel
  const displayYear = selectedHit?.year ?? value?.year
  const displayKind = value?.kind ?? selectedHit?.kind ?? kind
  const displayInLibrary =
    selectedHit?.source === 'library' || selectedHit?.libraryTitleId !== undefined
  const pick = (hit: TitleHit) => {
    setSelectedHit(hit)
    onChange(titleChoiceFromHit(hit))
    setOpen(false)
  }
  const useFree = () => {
    if (!freeTitle.trim()) return
    setSelectedHit(undefined)
    onChange({
      source: 'free',
      kind,
      title: freeTitle.trim(),
      ...(freeYear.trim() ? { year: Number(freeYear) } : {}),
    })
    setOpen(false)
  }
  return (
    <div className="flex flex-col gap-2">
      <span className="text-label font-bold tracking-label text-ink-2 uppercase">
        {t('assign.titlePicker.label')}
      </span>
      <Button
        variant="secondary"
        className="w-full min-w-0 justify-start text-left"
        onClick={() => {
          setOpen(true)
          setTerm(displayTitle ?? initialTerm)
        }}
      >
        {displayTitle ? (
          <span className="flex min-w-0 flex-1 items-center gap-3">
            {selectedHit?.posterFile ? (
              <img
                src={metadata.imageUrl('poster', selectedHit.posterFile)}
                alt=""
                className="size-12 shrink-0 rounded object-cover"
              />
            ) : (
              <span className="size-12 shrink-0 rounded bg-surface-2" aria-hidden="true" />
            )}
            <span className="min-w-0">
              <strong className="block truncate">{displayTitle}</strong>
              <span className="block text-caption text-ink-2">
                {[
                  displayYear,
                  kindLabel(displayKind, t),
                  displayInLibrary ? t('library.inLibrary') : '',
                ]
                  .filter(Boolean)
                  .join(` ${t('common.separator')} `)}
              </span>
            </span>
          </span>
        ) : (
          <span className="flex-1">
            {value?.source === 'library'
              ? t('assign.titlePicker.existing')
              : value?.source === 'tmdb'
                ? t('assign.titlePicker.tmdb')
                : t('assign.titlePicker.choose')}
          </span>
        )}
        {displayTitle && <span className="ml-auto shrink-0">{t('assign.titlePicker.change')}</span>}
      </Button>
      {error && <p className="m-0 text-body-sm text-danger">{error}</p>}
      {open && (
        <Dialog
          title={t('assign.titlePicker.dialogTitle')}
          subtitle={t('assign.titlePicker.dialogSubtitle')}
          onClose={() => {
            setOpen(false)
          }}
          initialFocus="field"
          size="lg"
          actions={
            <Button
              onClick={() => {
                setOpen(false)
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
          {search.data && !search.data.tmdbAvailable && (
            <p className="m-0 rounded-md border border-warning bg-warning-surface p-3 text-body-sm text-warning-ink">
              {t('assign.titlePicker.tmdbUnavailable')}
            </p>
          )}
          {!!search.data?.library.length && (
            <section className="flex flex-col gap-2">
              <h3 className="m-0 text-label font-bold tracking-label text-ink-2 uppercase">
                {t('assign.titlePicker.library')}
              </h3>
              {sortHits(search.data.library, kind).map((hit) => (
                <TitleHitButton
                  key={`library-${String(hit.libraryTitleId)}`}
                  hit={hit}
                  onPick={pick}
                />
              ))}
            </section>
          )}
          {!!search.data?.tmdb.length && (
            <section className="flex flex-col gap-2">
              <h3 className="m-0 text-label font-bold tracking-label text-ink-2 uppercase">
                {t('assign.titlePicker.tmdbResults')}
              </h3>
              {sortHits(search.data.tmdb, kind).map((hit) => (
                <TitleHitButton key={`tmdb-${String(hit.tmdbId)}`} hit={hit} onPick={pick} />
              ))}
            </section>
          )}
          <section className="flex flex-col gap-3 border-t border-line pt-4">
            <h3 className="m-0 text-label font-bold tracking-label text-ink-2 uppercase">
              {t('assign.titlePicker.free')}
            </h3>
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
          </section>
          <p className="m-0 text-caption text-ink-2">{t('attribution.tmdb')}</p>
        </Dialog>
      )}
    </div>
  )
}

function TitleHitButton({ hit, onPick }: { hit: TitleHit; onPick: (hit: TitleHit) => void }) {
  const { t } = useTranslation()
  const metadata = useMetadataPorts()
  return (
    <button
      type="button"
      className="flex min-h-control-md items-center gap-3 rounded-md border border-control p-3 text-left"
      onClick={() => {
        onPick(hit)
      }}
    >
      {hit.posterFile ? (
        <img
          src={metadata.imageUrl('poster', hit.posterFile)}
          alt=""
          className="size-12 shrink-0 rounded object-cover"
        />
      ) : (
        <span className="size-12 shrink-0 rounded bg-surface-2" aria-hidden="true" />
      )}
      <span className="min-w-0">
        <strong className="block truncate">{hit.title}</strong>
        <small className="text-ink-2">
          {[
            hit.year,
            kindLabel(hit.kind, t),
            hit.libraryTitleId !== undefined ? t('library.inLibrary') : '',
          ]
            .filter(Boolean)
            .join(` ${t('common.separator')} `)}
        </small>
      </span>
    </button>
  )
}
