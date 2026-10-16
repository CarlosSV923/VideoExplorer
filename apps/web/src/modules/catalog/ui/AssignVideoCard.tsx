import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useLetterShortcuts } from '@/shared/input'
import { NumberField, Select, TextField } from '@/shared/ui'
import type {
  AssignVideo,
  DuplicateDecision,
  PlanIssue,
  PlanVideo,
  VideoSuggestion,
} from '../domain/types'
import { DuplicateChoice } from './DuplicateChoice'
import { TitlePicker } from './TitlePicker'

export function AssignVideoCard({
  suggestion,
  value,
  plan,
  subtitles,
  showPart = false,
  showTitle = true,
  focused,
  onFocus,
  onMove,
  onChange,
  issues = [],
}: {
  suggestion: VideoSuggestion
  value: AssignVideo
  plan?: PlanVideo
  subtitles: number
  showPart?: boolean
  showTitle?: boolean
  focused: boolean
  onFocus: () => void
  onMove: (delta: number) => void
  onChange: (value: AssignVideo) => void
  issues?: readonly PlanIssue[]
}) {
  const { t } = useTranslation()
  const card = useRef<HTMLElement>(null)
  useLetterShortcuts(
    card,
    {
      r: () => {
        onChange({ ...value, duplicate: 'replace' })
      },
      a: () => {
        if (plan?.allowed.includes('keep_both')) onChange({ ...value, duplicate: 'keep_both' })
      },
      o: () => {
        onChange({ ...value, duplicate: 'skip' })
      },
    },
    focused,
  )
  const reading = suggestion.reading
  const issueText = (code: string) =>
    t(`assign.issue.${code}`, { defaultValue: t('assign.issue.generic') })
  const fieldError = (field: string) => {
    const issue = issues.find((item) => item.field === field)
    return issue ? issueText(issue.code) : undefined
  }
  const fileError = fieldError('file') ?? fieldError('video') ?? fieldError('subtitle')
  const [selectedKind, setSelectedKind] = useState<'movie' | 'series'>(
    value.title?.kind ?? (reading.type === 'series' ? 'series' : 'movie'),
  )
  const series = selectedKind === 'series'
  const freeSeries = value.title?.source === 'free' && series
  const change = (patch: Partial<AssignVideo>) => {
    onChange({ ...value, ...patch })
  }
  const changeNumber = (field: 'season' | 'chapter' | 'lastChapter' | 'part', raw: string) => {
    if (raw) {
      change({ [field]: Number(raw) })
      return
    }
    const next = { ...value }
    if (field === 'season') delete next.season
    if (field === 'chapter') delete next.chapter
    if (field === 'lastChapter') delete next.lastChapter
    if (field === 'part') delete next.part
    onChange(next)
  }
  const whatIs = value.action === 'keep' ? 'keep' : series ? 'episode' : 'movie'
  const changeWhatIs = (next: string) => {
    if (next === 'keep') {
      change({ action: 'keep' })
      return
    }
    const nextKind = next === 'episode' ? 'series' : 'movie'
    setSelectedKind(nextKind)
    change({
      action: 'place',
      ...(value.title ? { title: { ...value.title, kind: nextKind } } : {}),
    })
  }
  return (
    <article
      ref={card}
      tabIndex={0}
      data-video-card="true"
      className="flex flex-col gap-4 rounded-lg border border-line bg-surface p-5 outline-none focus-visible:border-accent focus-visible:ring-2 focus-visible:ring-accent"
      onFocus={onFocus}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        if (event.key === 'ArrowUp') {
          event.preventDefault()
          onMove(-1)
        }
        if (event.key === 'ArrowDown') {
          event.preventDefault()
          onMove(1)
        }
        if (event.key === 'Enter') {
          event.preventDefault()
          onChange({ ...value, action: 'place' })
        }
      }}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="m-0 break-all font-mono-file text-mono text-ink-1">
            {suggestion.file || t('assign.video.loose')}
          </p>
          <p className="m-0 mt-1 text-caption text-ink-2">
            {reading.type === 'unknown'
              ? t('assign.video.unknownType')
              : t(`assign.video.type.${reading.type}`)}
          </p>
          {fileError && <p className="m-0 mt-1 text-body-sm text-danger">{fileError}</p>}
        </div>
        <Select
          label={t('assign.video.whatIs')}
          value={whatIs}
          {...(fieldError('kind') || fieldError('action') || fieldError('place')
            ? { error: fieldError('kind') ?? fieldError('action') ?? fieldError('place') }
            : {})}
          onChange={(event) => {
            changeWhatIs(event.currentTarget.value)
          }}
        >
          <option value="movie">{t('assign.video.whatIsMovie')}</option>
          <option value="episode">{t('assign.video.whatIsEpisode')}</option>
          <option value="keep">{t('assign.video.whatIsKeep')}</option>
        </Select>
      </div>
      {value.action === 'place' && (
        <>
          {showTitle && (
            <TitlePicker
              {...(value.title ? { value: value.title } : {})}
              kind={series ? 'series' : 'movie'}
              {...(suggestion.match ? { suggested: suggestion.match } : {})}
              {...(reading.title !== undefined ? { initialTerm: reading.title } : {})}
              {...(fieldError('title') ? { error: fieldError('title') } : {})}
              onChange={(title) => {
                setSelectedKind(title.kind ?? 'movie')
                change({ title })
              }}
            />
          )}
          {series ? (
            <div className="grid gap-3 sm:grid-cols-3">
              <NumberField
                label={t('assign.video.season')}
                {...(fieldError('season') ? { error: fieldError('season') } : {})}
                min={0}
                value={value.season ?? reading.season ?? ''}
                onChange={(event) => {
                  changeNumber('season', event.currentTarget.value)
                }}
              />
              <NumberField
                label={t('assign.video.chapter')}
                {...(fieldError('chapter') ? { error: fieldError('chapter') } : {})}
                min={1}
                value={value.chapter ?? reading.chapter ?? ''}
                onChange={(event) => {
                  changeNumber('chapter', event.currentTarget.value)
                }}
              />
              <NumberField
                label={t('assign.video.lastChapter')}
                {...(fieldError('lastChapter') ? { error: fieldError('lastChapter') } : {})}
                min={1}
                value={value.lastChapter ?? reading.lastChapter ?? ''}
                onChange={(event) => {
                  changeNumber('lastChapter', event.currentTarget.value)
                }}
              />
            </div>
          ) : showPart ? (
            <NumberField
              label={t('assign.video.part')}
              {...(fieldError('part') ? { error: fieldError('part') } : {})}
              min={1}
              value={value.part ?? reading.part ?? ''}
              onChange={(event) => {
                changeNumber('part', event.currentTarget.value)
              }}
            />
          ) : null}
          {freeSeries && (
            <TextField
              label={t('assign.video.chapterTitle')}
              value={value.chapterTitle ?? ''}
              onChange={(event) => {
                change({ chapterTitle: event.currentTarget.value })
              }}
            />
          )}
          {subtitles > 0 && (
            <span className="text-caption text-ink-2">
              {t('assign.subtitles.count', { count: subtitles })}
            </span>
          )}
        </>
      )}
      <DuplicateChoice
        plan={plan}
        error={fieldError('duplicate')}
        {...(value.duplicate ? { value: value.duplicate } : {})}
        onChange={(duplicate: DuplicateDecision) => {
          change({ duplicate })
        }}
      />
      {plan?.versionIncomplete && (
        <TextField
          label={t('assign.duplicate.version')}
          value={value.version ?? plan.versionProposal ?? ''}
          error={fieldError('version') ?? t('assign.duplicate.incomplete')}
          onChange={(event) => {
            change({ version: event.currentTarget.value })
          }}
        />
      )}
    </article>
  )
}
