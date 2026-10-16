import { describe, expect, it } from 'vitest'

import {
  initialSubtitleRequest,
  saveState,
  subtitlesByVideo,
  titleChoiceFromHit,
} from './assignment'
import type { AssignPlan } from '../domain/types'

const plan = (overrides: Partial<AssignPlan> = {}): AssignPlan => ({
  entryId: 1,
  valid: true,
  videos: [],
  subtitles: [],
  errors: [],
  warnings: [],
  ...overrides,
})

describe('assignment application rules', () => {
  it('blocks saving until there is a valid preview', () => {
    expect(saveState(null)).toEqual({ enabled: false, reason: 'noPreview' })
    expect(
      saveState(plan({ valid: false, errors: [{ field: 'duplicate', code: 'required' }] })),
    ).toEqual({
      enabled: false,
      reason: 'duplicate',
    })
    expect(saveState(plan())).toEqual({ enabled: true })
  })

  it('groups subtitles by their associated video and keeps unassigned files visible', () => {
    const grouped = subtitlesByVideo([
      { file: 'one.es.srt', video: 'one.mkv', language: 'es-419', forced: false, sdh: false },
      { file: 'orphan.srt', language: 'en', forced: false, sdh: true },
      { file: 'one.en.srt', video: 'one.mkv', language: 'en', forced: false, sdh: false },
    ])
    expect(grouped.get('one.mkv')?.map((subtitle) => subtitle.file)).toEqual([
      'one.es.srt',
      'one.en.srt',
    ])
    expect(grouped.get('')).toHaveLength(1)
  })

  it('defaults subtitle language to Latin American Spanish when the adapter omits it', () => {
    expect(
      initialSubtitleRequest([
        { file: 'movie.srt', video: 'movie.mkv', language: '', forced: false, sdh: false },
      ]),
    ).toEqual([
      { file: 'movie.srt', video: 'movie.mkv', language: 'es-419', forced: false, sdh: false },
    ])
  })

  it('assigns a TMDB hit already in the library by library title id', () => {
    expect(
      titleChoiceFromHit({
        source: 'tmdb',
        kind: 'series',
        tmdbId: 113988,
        libraryTitleId: 3,
        title: 'The Bear',
        year: 2022,
      }),
    ).toEqual({ source: 'library', kind: 'series', libraryTitleId: 3 })
  })
})
