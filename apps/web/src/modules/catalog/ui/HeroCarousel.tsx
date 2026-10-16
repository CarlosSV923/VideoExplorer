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
