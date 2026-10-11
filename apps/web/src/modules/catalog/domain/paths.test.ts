import vectors from '../../../../../../contracts/naming-cases.json'
import { chapterTitle, discTitle, type ChapterNames } from '../../metadata/domain/naming'

import { NameError } from './errors'
import { chapterPath, defaultIdTag, moviePath, subtitlePath, type Title } from './paths'

interface ChapterCase {
  original?: string
  english?: string
}

interface PathInput {
  kind: string
  roots?: { movies?: string; series?: string }
  original?: string
  english?: string
  year?: number
  tmdbId?: number
  idTag?: boolean
  extension: string
  version?: string
  part?: number
  season?: number
  episode?: number
  episodeEnd?: number
  chapters?: ChapterCase[]
  video?: PathInput
  language?: string
  forced?: boolean
  sdh?: boolean
}

interface PathCase {
  name: string
  input: PathInput
  expected: string | { error: string }
}

/**
 * The path a case describes, as its caller builds it: the disc title and the chapter title come
 * from metadata (TMDB's titles, spec §5), then the library names are built from them.
 */
function build(input: PathInput): string {
  switch (input.kind) {
    case 'movie':
      return moviePath({
        root: input.roots?.movies || 'Movies',
        title: titleOf(input),
        version: input.version ?? '',
        part: input.part ?? 0,
        extension: input.extension,
      })
    case 'episode': {
      const first = input.episode ?? 0
      return chapterPath({
        root: input.roots?.series || 'TV Shows',
        title: titleOf(input),
        season: input.season ?? 0,
        first,
        last: input.episodeEnd ?? first,
        chapterTitle: chapterTitleOf(input),
        version: input.version ?? '',
        extension: input.extension,
      })
    }
    case 'subtitle': {
      if (input.video === undefined) throw new Error('a subtitle case needs its video')
      const video = build(input.video)
      return subtitlePath(video, {
        language: input.language ?? '',
        forced: input.forced ?? false,
        sdh: input.sdh ?? false,
        extension: input.extension,
      })
    }
    default:
      throw new Error(`unknown kind ${input.kind}`)
  }
}

/** The title of a case. The tag is on by default for a linked title (RF-45). */
function titleOf(input: PathInput): Title {
  const tmdbId = input.tmdbId ?? 0
  return {
    name: discTitle(input.original ?? '', input.english ?? ''),
    year: input.year ?? 0,
    tmdbId,
    idTag: input.idTag ?? defaultIdTag(tmdbId),
  }
}

function chapterTitleOf(input: PathInput): string {
  const chapters: ChapterNames[] = (input.chapters ?? []).map((c) => ({
    original: c.original ?? '',
    english: c.english ?? '',
  }))
  return chapterTitle(chapters)
}

/** Runs fn and returns what it threw. */
function thrown(fn: () => unknown): unknown {
  try {
    fn()
  } catch (err) {
    return err
  }
  return undefined
}

describe('library path vectors (contracts/naming-cases.json)', () => {
  it.each((vectors.paths.cases as PathCase[]).map((c) => [c.name, c] as const))(
    'path: %s',
    (_, c) => {
      if (typeof c.expected === 'string') {
        expect(build(c.input)).toBe(c.expected)
        return
      }
      const err = thrown(() => build(c.input))
      expect(err).toBeInstanceOf(NameError)
      expect((err as NameError).field).toBe(c.expected.error)
    },
  )
})

describe('library path rules', () => {
  it('a linked title has its tag by default and a free title never has one', () => {
    expect(defaultIdTag(1726)).toBe(true)
    expect(defaultIdTag(0)).toBe(false)
  })

  it.each([
    [
      'a movie title over 255 bytes is cut and the tag stays',
      moviePath({
        root: 'Movies',
        title: { name: 'é'.repeat(200), year: 2008, tmdbId: 1, idTag: true },
        version: '',
        part: 0,
        extension: '.mkv',
      }),
      ' (2008) [tmdbid-1]/',
    ],
    [
      'a chapter title over 255 bytes is cut and the codes stay',
      chapterPath({
        root: 'TV Shows',
        title: { name: 'Breaking Bad', year: 2008, tmdbId: 1396, idTag: true },
        season: 1,
        first: 1,
        last: 1,
        chapterTitle: 'é'.repeat(200),
        version: '',
        extension: '.mkv',
      }),
      ' - S01E01 - ',
    ],
  ])('%s', (_, got, mustContain) => {
    expect(got).toContain(mustContain)
    for (const component of got.split('/')) {
      expect(new TextEncoder().encode(component).length).toBeLessThanOrEqual(255)
    }
  })

  it('a subtitle keeps its language and flags when the video name is cut', () => {
    const video = 'Movies/' + 'a'.repeat(250) + '/' + 'a'.repeat(250) + '.mkv'
    const got = subtitlePath(video, {
      language: 'es-MX',
      forced: true,
      sdh: false,
      extension: '.srt',
    })
    expect(got.endsWith('.es-419.forced.srt')).toBe(true)
    for (const component of got.split('/')) {
      expect(new TextEncoder().encode(component).length).toBeLessThanOrEqual(255)
    }
  })

  it('a movie without a name is rejected', () => {
    const err = thrown(() =>
      moviePath({
        root: 'Movies',
        title: { name: '', year: 2008, tmdbId: 0, idTag: false },
        version: '',
        part: 0,
        extension: '.mkv',
      }),
    )
    expect(err).toBeInstanceOf(NameError)
    expect((err as NameError).field).toBe('title')
  })
})
