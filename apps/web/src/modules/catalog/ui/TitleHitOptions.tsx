import { useId, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import type { TitleHit } from '../domain/types'
import { useMetadataPorts } from '@/modules/metadata/application/ports'
import { useOptionalAction } from '@/shared/input'
import { CheckIcon } from '@/shared/ui/icons'
import { kindLabel } from './CatalogLabels'

function hitKey(hit: TitleHit) {
  if (hit.libraryTitleId !== undefined) return `library:${String(hit.libraryTitleId)}`
  return `${hit.kind}:${String(hit.tmdbId ?? '')}`
}

export function TitleHitOptions({
  library,
  tmdb,
  selected,
  onPick,
}: {
  library: readonly TitleHit[]
  tmdb: readonly TitleHit[]
  selected?: TitleHit | null | undefined
  onPick: (hit: TitleHit) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const [activeKey, setActiveKey] = useState<string>()
  const [shown, setShown] = useState<Record<string, number>>({})
  const selectedKey = selected ? hitKey(selected) : undefined
  const libraryKeys = new Set(library.map(hitKey))
  const tmdbOptions = tmdb.filter((hit) => !libraryKeys.has(hitKey(hit)))
  const sections = [
    { key: 'library', title: t('assign.titlePicker.library'), hits: library },
    { key: 'tmdb', title: t('assign.titlePicker.tmdbResults'), hits: tmdbOptions },
  ] as const
  const focusOption = (current: HTMLButtonElement, direction: string) => {
    const options = Array.from(
      root.current?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [],
    )
    const index = options.indexOf(current)
    const delta = direction === 'up' || direction === 'left' ? -1 : 1
    const next = options[(index + delta + options.length) % options.length]
    next?.focus()
  }
  useOptionalAction('navigate', (direction) => {
    if (!direction || !root.current?.contains(document.activeElement)) return false
    const current = document.activeElement
    if (!(current instanceof HTMLButtonElement) || current.getAttribute('role') !== 'radio')
      return false
    focusOption(current, direction)
    return true
  })
  const onOptionKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(event.key)) return
    event.preventDefault()
    focusOption(event.currentTarget, event.key.replace('Arrow', '').toLowerCase())
  }

  return (
    <div ref={root} data-title-options className="flex flex-col gap-4">
      {sections.map(({ key, title, hits }) => {
        if (hits.length === 0) return null
        const sorted = [...hits]
        const preferredSelected = selectedKey
          ? sorted.findIndex((hit) => hitKey(hit) === selectedKey)
          : -1
        const firstHit = library[0] ?? tmdbOptions[0]
        const focusKey = activeKey ?? selectedKey ?? (firstHit ? hitKey(firstHit) : undefined)
        const visibleCount = Math.max(5, preferredSelected + 1, shown[key] ?? 0)
        const visible = sorted.slice(0, visibleCount)
        const remaining = sorted.length - visible.length
        const labelId = `${id}-${key}-instruction`
        const sectionId = `${id}-${key}-section`
        return (
          <section key={key} className="flex flex-col gap-2">
            <h3 id={labelId} className="m-0 text-label font-bold text-ink-1">
              {t('assign.titlePicker.chooseMatch')}
            </h3>
            <p
              id={sectionId}
              className="m-0 text-caption font-bold tracking-label text-ink-2 uppercase"
            >
              {title}
            </p>
            <div
              role="radiogroup"
              aria-labelledby={`${labelId} ${sectionId}`}
              className="flex flex-col gap-2"
            >
              {visible.map((hit) => {
                const isSelected = hitKey(hit) === selectedKey
                const keyForHit = hitKey(hit)
                const tabIndex = keyForHit === focusKey ? 0 : -1
                return (
                  <TitleHitOption
                    key={`${key}-${keyForHit}`}
                    hit={hit}
                    selected={isSelected}
                    tabIndex={tabIndex}
                    onFocus={() => {
                      setActiveKey(keyForHit)
                    }}
                    onKeyDown={onOptionKeyDown}
                    onPick={() => {
                      setActiveKey(keyForHit)
                      onPick(hit)
                    }}
                  />
                )
              })}
            </div>
            {remaining > 0 && (
              <button
                type="button"
                className="min-h-control-sm self-start text-body-sm font-semibold text-accent underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-accent"
                onClick={() => {
                  setShown((current) => ({
                    ...current,
                    [key]: visible.length + Math.min(5, remaining),
                  }))
                }}
              >
                {t('assign.titlePicker.showMore', { count: Math.min(5, remaining) })}
              </button>
            )}
          </section>
        )
      })}
    </div>
  )
}

function TitleHitOption({
  hit,
  selected,
  tabIndex,
  onFocus,
  onKeyDown,
  onPick,
}: {
  hit: TitleHit
  selected: boolean
  tabIndex: number
  onFocus: () => void
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void
  onPick: () => void
}) {
  const { t } = useTranslation()
  const metadata = useMetadataPorts()
  const [imageFailed, setImageFailed] = useState(false)
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      tabIndex={tabIndex}
      onFocus={onFocus}
      onKeyDown={onKeyDown}
      onClick={onPick}
      className={`flex min-h-[88px] w-full min-w-0 cursor-pointer items-center gap-3 rounded-md border-2 p-2 text-left transition-colors ${selected ? 'border-accent bg-success-surface' : 'border-control bg-surface-2 hover:bg-surface-3'}`}
    >
      <span
        aria-hidden="true"
        className={`grid size-5 shrink-0 place-items-center rounded-full border-2 ${selected ? 'border-accent bg-accent' : 'border-control'}`}
      >
        {selected && <span className="size-2 rounded-full bg-on-accent" />}
      </span>
      {hit.posterFile && !imageFailed ? (
        <img
          src={metadata.imageUrl('poster', hit.posterFile)}
          alt=""
          loading="lazy"
          onError={() => {
            setImageFailed(true)
          }}
          className="h-[60px] w-[40px] shrink-0 rounded-sm object-cover md:h-[72px] md:w-[48px]"
        />
      ) : (
        <PosterPlaceholder title={hit.title} />
      )}
      <span className="min-w-0 flex-1">
        <strong className="line-clamp-2 block font-semibold text-ink-1">{hit.title}</strong>
        <small className="block text-ink-2">
          {[hit.year || undefined, kindLabel(hit.kind, t)]
            .filter(Boolean)
            .join(` ${t('common.separator')} `)}
        </small>
        {hit.libraryTitleId !== undefined && (
          <small className="mt-1 inline-flex rounded-full bg-surface-3 px-2 py-0.5 text-caption text-ink-1">
            {t('library.inLibrary')}
          </small>
        )}
      </span>
      {selected && (
        <span className="ml-auto inline-flex shrink-0 items-center gap-1 text-caption font-semibold text-accent">
          <CheckIcon size={16} />
          {t('assign.titlePicker.chosen')}
        </span>
      )}
    </button>
  )
}

function PosterPlaceholder({ title }: { title: string }) {
  let hash = 0
  for (const character of title) hash = (hash * 31 + (character.codePointAt(0) ?? 0)) % 360
  return (
    <span
      aria-hidden="true"
      className="flex h-[60px] w-[40px] shrink-0 items-end overflow-hidden rounded-sm p-1 text-caption font-semibold text-ink-1 md:h-[72px] md:w-[48px] md:p-1.5"
      style={{
        background: `linear-gradient(165deg, hsl(${String(hash)} 45% 34%), hsl(${String((hash + 42) % 360)} 42% 12%))`,
      }}
    >
      <span className="line-clamp-3">{title}</span>
    </span>
  )
}
