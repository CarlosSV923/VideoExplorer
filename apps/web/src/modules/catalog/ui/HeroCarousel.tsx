import { useQueries } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '../application/ports'
import type { LibraryTitleSummary } from '../domain/types'
import {
  ArrowRightIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  EditIcon,
  PauseIcon,
  PlayIcon,
} from '@/shared/ui/icons'
import { IconButton, LinkButton } from '@/shared/ui'
import { demoImage } from '@/modules/metadata/infrastructure/demo/metadataDemo'

function useReducedMotion() {
  const [reduced, setReduced] = useState(
    () => window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  )
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

export function HeroCarousel({ titles }: { titles: LibraryTitleSummary[] }) {
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
      className="relative -mx-[var(--page-x,16px)] h-[440px] overflow-hidden md:h-[600px]"
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
        <img
          src={current.backdrop}
          alt=""
          className="absolute inset-0 size-full object-cover"
          onError={(event) => {
            event.currentTarget.src = demoImage(current.title, 'backdrop')
          }}
        />
      )}
      <div className="absolute inset-0 bg-scrim" />
      <div className="relative flex h-full max-w-3xl flex-col justify-end gap-2 px-[var(--page-x,16px)] py-6 md:gap-4 md:py-12">
        <span className="text-label font-bold uppercase tracking-label text-accent">
          {t('library.featured')}
        </span>
        <h2 className="m-0 flex h-[110px] w-[min(300px,70vw)] max-w-full shrink-0 items-end overflow-hidden text-display-hero font-display uppercase md:h-40 md:w-[min(520px,70vw)]">
          {detail?.logo ? (
            <img
              src={detail.logo}
              alt={detail.title}
              className="size-full object-contain object-left-bottom"
              onError={(event) => {
                event.currentTarget.src = demoImage(current.title, 'logo')
              }}
            />
          ) : (
            (detail?.title ?? current.title)
          )}
        </h2>
        <p className="m-0 h-[42px] max-h-[42px] shrink-0 overflow-hidden text-body-sm text-ink-2">
          {detail?.year ?? current.year}
          {(detail?.genres ?? []).length ? ` · ${(detail?.genres ?? []).join(' · ')}` : ''}
        </p>
        <p className="m-0 h-[81px] max-h-[81px] max-w-2xl shrink-0 overflow-hidden text-body-lg text-ink-1 line-clamp-3">
          {detail?.overview ?? ''}
        </p>
        <div className="flex h-12 shrink-0 flex-wrap gap-3">
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
          className="flex h-11 shrink-0 items-center gap-2"
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
