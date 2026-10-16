import { useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from './cx'
import { ChevronLeftIcon, ChevronRightIcon } from './icons'
import { IconButton } from './Button'

export interface PosterProps {
  title: string
  meta?: string
  image?: string
  chips?: readonly ReactNode[]
  accessibleChips?: readonly string[]
  href?: string
  dimmed?: boolean
  seed?: string
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
          <span className="absolute top-2 right-2 flex max-w-[calc(100%-16px)] flex-wrap justify-end gap-1">
            {chips.map((chip, index) => (
              <span key={index}>{chip}</span>
            ))}
          </span>
        )}
      </div>
      {meta && <span className="mt-2 block text-caption text-ink-2">{meta}</span>}
    </>
  )
  const accessibleName = [title, meta, ...accessibleChips].filter(Boolean).join(', ')
  return href ? (
    <a
      href={href}
      aria-label={accessibleName}
      className="group block min-w-0 w-poster-w no-underline transition duration-120 ease-out hover:-translate-y-1 focus-visible:-translate-y-1"
    >
      {content}
    </a>
  ) : (
    <article aria-label={accessibleName} className="block min-w-0 w-poster-w">
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
          className="flex min-w-0 gap-4 overflow-x-auto scroll-smooth snap-x snap-mandatory px-12 pb-2"
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
