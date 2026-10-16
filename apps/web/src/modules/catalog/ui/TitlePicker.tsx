import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useTitleSearch } from '@/modules/metadata/application/queries'
import type { TitleChoice, TitleHit } from '../domain/types'
import { Button, Dialog, TextField } from '@/shared/ui'

function choiceFromHit(hit: TitleHit): TitleChoice {
  return {
    source: hit.source,
    kind: hit.kind,
    ...(hit.libraryTitleId !== undefined ? { libraryTitleId: hit.libraryTitleId } : {}),
    ...(hit.tmdbId !== undefined ? { tmdbId: hit.tmdbId } : {}),
  }
}

export function TitlePicker({
  value,
  initialTerm = '',
  kind = 'movie',
  onChange,
}: {
  value?: TitleChoice
  initialTerm?: string
  kind?: 'movie' | 'series'
  onChange: (value: TitleChoice) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [term, setTerm] = useState(initialTerm)
  const [freeTitle, setFreeTitle] = useState('')
  const [freeYear, setFreeYear] = useState('')
  const search = useTitleSearch(term)
  const selectedLabel = value?.source === 'free' ? value.title : undefined
  const pick = (hit: TitleHit) => {
    onChange(choiceFromHit(hit))
    setOpen(false)
  }
  const useFree = () => {
    if (!freeTitle.trim()) return
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
        className="justify-start text-left"
        onClick={() => {
          setOpen(true)
          setTerm(selectedLabel ?? initialTerm)
        }}
      >
        {selectedLabel ??
          (value?.source === 'library'
            ? t('assign.titlePicker.existing')
            : value?.source === 'tmdb'
              ? t('assign.titlePicker.tmdb')
              : t('assign.titlePicker.choose'))}
      </Button>
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
              {search.data.library.map((hit) => (
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
              {search.data.tmdb.map((hit) => (
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
  return (
    <button
      type="button"
      className="flex min-h-control-md items-center gap-3 rounded-md border border-control p-3 text-left"
      onClick={() => {
        onPick(hit)
      }}
    >
      <span className="size-12 shrink-0 rounded bg-surface-2" aria-hidden="true" />
      <span className="min-w-0">
        <strong className="block truncate">{hit.title}</strong>
        <small className="text-ink-2">
          {hit.year ?? ''} · {hit.kind}
        </small>
      </span>
    </button>
  )
}
