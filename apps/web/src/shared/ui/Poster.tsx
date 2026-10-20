import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from './cx'
import { ChevronLeftIcon, ChevronRightIcon } from './icons'
import { Button, IconButton } from './Button'

export interface PosterProps {
  title: string
  meta?: string
  image?: string
  chips?: readonly ReactNode[]
  accessibleChips?: readonly string[]
  href?: string
  dimmed?: boolean
  seed?: string
  fluid?: boolean
}

function generatedBackground(seed: string) {
  let hash = 0
  for (const character of seed) hash = (hash * 31 + (character.codePointAt(0) ?? 0)) % 360
  return {
    background: `linear-gradient(165deg, hsl(${String(hash)} 45% 34%), hsl(${String((hash + 42) % 360)} 42% 12%))`,
  }
}

export function Poster({
  title,
  meta,
  image,
  chips = [],
  accessibleChips = [],
  href,
  dimmed = false,
  seed = title,
  fluid = false,
}: PosterProps) {
  const content = (
    <>
      <div
        className={cx(
          'relative aspect-[2/3] overflow-hidden rounded-md bg-surface-missing',
          dimmed && 'bg-surface-missing',
        )}
        style={image ? undefined : generatedBackground(seed)}
      >
        {image ? (
          <img
            src={image}
            alt=""
            className={cx('size-full object-cover', dimmed && 'brightness-55')}
            onError={(event) => {
              if (event.currentTarget.dataset.fallback) return
              event.currentTarget.dataset.fallback = 'true'
              const fallback = document.createElement('span')
              fallback.setAttribute('aria-hidden', 'true')
              fallback.className =
                'absolute inset-3 flex items-end text-body-sm font-display uppercase leading-tight text-ink-1'
              fallback.textContent = title
              event.currentTarget.replaceWith(fallback)
              const parent = fallback.parentElement
              if (parent) parent.style.background = generatedBackground(seed).background
            }}
          />
        ) : (
          <span
            aria-hidden="true"
            className="absolute inset-3 flex items-end text-body-sm font-display uppercase leading-tight text-ink-1"
          >
            {title}
          </span>
        )}
        {chips.length > 0 && (
          <span className="absolute top-2 right-2 flex max-w-[calc(100%-16px)] flex-wrap justify-end gap-1 [&>span>span]:max-w-full [&>span>span]:whitespace-normal [&>span>span]:break-words">
            {chips.map((chip, index) => (
              <span key={index}>{chip}</span>
            ))}
          </span>
        )}
      </div>
      <span className="mt-2 block line-clamp-3 break-normal text-body-sm font-semibold text-ink-1">
        {title}
      </span>
      {meta && <span className="block text-caption text-ink-2">{meta}</span>}
    </>
  )
  const accessibleName = [title, meta, ...accessibleChips].filter(Boolean).join(', ')
  return href ? (
    <a
      href={href}
      aria-label={accessibleName}
      className={cx(
        'group block min-w-0 shrink-0 snap-start no-underline transition duration-120 ease-out hover:-translate-y-1 focus-visible:-translate-y-1',
        fluid ? 'w-full' : 'w-poster-w',
      )}
    >
      {content}
    </a>
  ) : (
    <article
      aria-label={accessibleName}
      className={cx('block min-w-0 shrink-0', fluid ? 'w-full' : 'w-poster-w')}
    >
      {content}
    </article>
  )
}

export function PosterRow({
  title,
  count,
  seeAll,
  children,
}: {
  title: string
  count?: number
  seeAll?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  const row = useRef<HTMLDivElement>(null)
  const scroll = (direction: number) =>
    row.current?.scrollBy({
      left: direction * Math.max(240, row.current.clientWidth - 176),
      behavior: 'smooth',
    })
  return (
    <section aria-labelledby={`poster-row-${title}`} className="flex min-w-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <h2 id={`poster-row-${title}`} className="m-0 text-heading font-display">
          {title}
          {count !== undefined && (
            <span className="ml-2 text-body-sm font-medium text-ink-3">{count}</span>
          )}
        </h2>
        {seeAll}
      </div>
      <div className="relative min-w-0">
        <IconButton
          label={t('common.previous')}
          className="absolute top-1/2 left-1 z-10 -translate-y-1/2 bg-surface/90"
          onClick={() => scroll(-1)}
        >
          <ChevronLeftIcon />
        </IconButton>
        <div
          ref={row}
          className="flex min-w-0 gap-3 overflow-x-auto scroll-smooth snap-x snap-mandatory px-12 pb-2 md:gap-4"
        >
          {children}
        </div>
        <IconButton
          label={t('common.next')}
          className="absolute top-1/2 right-1 z-10 -translate-y-1/2 bg-surface/90"
          onClick={() => scroll(1)}
        >
          <ChevronRightIcon />
        </IconButton>
      </div>
    </section>
  )
}

export function AutoPosterRow({
  title,
  items,
  seeAll,
}: {
  title: string
  items: ReactNode[]
  seeAll: ReactNode
}) {
  const { t } = useTranslation()
  const row = useRef<HTMLDivElement>(null)
  const posters = items.slice(0, 10)
  const [manualPaused, setManualPaused] = useState(false)
  const [pointerInside, setPointerInside] = useState(false)
  const [focusInside, setFocusInside] = useState(false)
  const [reducedMotion, setReducedMotion] = useState(
    () =>
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  )
  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const preference = window.matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => {
      setReducedMotion(preference.matches)
    }
    update()
    preference.addEventListener('change', update)
    return () => {
      preference.removeEventListener('change', update)
    }
  }, [])
  const paused = manualPaused || pointerInside || focusInside || reducedMotion

  useEffect(() => {
    if (paused || posters.length < 2) return
    const timer = window.setInterval(() => {
      const element = row.current
      const firstPoster = element?.firstElementChild
      if (!element || !firstPoster) return
      const style = window.getComputedStyle(element)
      const gap = Number.parseFloat(style.columnGap || style.gap) || 0
      const step = firstPoster.getBoundingClientRect().width + gap
      const max = element.scrollWidth - element.clientWidth
      if (element.scrollLeft + step >= max - 1) {
        element.scrollTo({ left: 0, behavior: 'smooth' })
      } else {
        element.scrollBy({ left: step, behavior: 'smooth' })
      }
    }, 4000)
    return () => {
      window.clearInterval(timer)
    }
  }, [posters.length, paused])

  const scroll = (direction: number) => {
    const element = row.current
    if (!element) return
    const firstPoster = element.firstElementChild
    const style = window.getComputedStyle(element)
    const gap = Number.parseFloat(style.columnGap || style.gap) || 0
    const step = (firstPoster?.getBoundingClientRect().width ?? 176) + gap
    element.scrollBy({
      left: direction * Math.max(step, element.clientWidth - step),
      behavior: 'smooth',
    })
  }

  return (
    <section
      aria-labelledby={`poster-row-${title}`}
      className="flex min-w-0 flex-col gap-3"
      onMouseEnter={() => {
        setPointerInside(true)
      }}
      onMouseLeave={() => {
        setPointerInside(false)
      }}
      onFocusCapture={() => {
        setFocusInside(true)
      }}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setFocusInside(false)
      }}
    >
      <div className="flex flex-col items-start justify-between gap-2 sm:flex-row sm:items-center sm:gap-4">
        <h2 id={`poster-row-${title}`} className="m-0 text-heading font-display">
          {title}
        </h2>
        <div className="flex shrink-0 items-center gap-2 self-end sm:self-auto">
          {seeAll}
          <IconButton
            label={t('common.previous')}
            onClick={() => {
              scroll(-1)
            }}
          >
            <ChevronLeftIcon />
          </IconButton>
          <Button
            size="sm"
            aria-label={t(paused ? 'library.resumeAutoRowLabel' : 'library.pauseAutoRowLabel')}
            aria-pressed={paused}
            onClick={() => {
              setManualPaused(paused ? false : true)
            }}
          >
            {t(paused ? 'library.resumeAutoRow' : 'library.pauseAutoRow')}
          </Button>
          <IconButton
            label={t('common.next')}
            onClick={() => {
              scroll(1)
            }}
          >
            <ChevronRightIcon />
          </IconButton>
        </div>
      </div>
      <div
        className="relative min-w-0"
        onTouchStart={() => {
          setManualPaused(true)
        }}
        onWheel={(event) => {
          const delta =
            Math.abs(event.deltaY) > Math.abs(event.deltaX) ? event.deltaY : event.deltaX
          if (delta !== 0) {
            event.preventDefault()
            row.current?.scrollBy({ left: delta, behavior: 'auto' })
          }
        }}
      >
        <div
          ref={row}
          data-auto-poster-row
          className="auto-poster-row flex min-w-0 gap-3 overflow-x-auto scroll-smooth snap-x snap-mandatory px-12 pb-2 md:gap-4"
        >
          {posters}
        </div>
      </div>
    </section>
  )
}
