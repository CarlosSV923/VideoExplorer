import { afterEach, describe, expect, it } from 'vitest'

import { createCatalogDemo } from './catalogDemo'
import type { AssignRequest, LibraryTitle } from '@/modules/catalog/domain/types'

describe('catalog demo Jellyfin assignment paths', () => {
  const cases = [
    {
      name: 'movie',
      entryId: 2,
      request: (): AssignRequest => ({
        videos: [
          {
            file: 'Mufasa.The.Lion.King.2024.2160p.mkv',
            action: 'place',
            title: { source: 'tmdb', tmdbId: 762509, kind: 'movie' },
          },
        ],
        subtitles: [],
      }),
      expected:
        'Movies/Mufasa - The Lion King (2024) [tmdbid-762509]/Mufasa - The Lion King (2024) [tmdbid-762509].mkv',
    },
    {
      name: 'movie part',
      entryId: 2,
      request: (): AssignRequest => ({
        videos: [
          {
            file: 'Mufasa.The.Lion.King.2024.2160p.mkv',
            action: 'place',
            title: { source: 'tmdb', tmdbId: 762509, kind: 'movie' },
            part: 2,
          },
        ],
        subtitles: [],
      }),
      expected:
        'Movies/Mufasa - The Lion King (2024) [tmdbid-762509]/Mufasa - The Lion King (2024) [tmdbid-762509] - part2.mkv',
    },
    {
      name: 'movie version',
      entryId: 4,
      request: (title: LibraryTitle | undefined): AssignRequest => ({
        videos: [
          {
            file: 'The.Dark.Knight.2008.1080p.mkv',
            action: 'place',
            title: {
              source: 'library',
              ...(title ? { libraryTitleId: title.id } : {}),
              kind: 'movie',
            },
            duplicate: 'keep_both',
            version: '1080p',
          },
        ],
        subtitles: [],
      }),
      expected:
        'Movies/The Dark Knight (2008) [tmdbid-155]/The Dark Knight (2008) [tmdbid-155] - 1080p.mkv',
      title: 'The Dark Knight',
    },
    {
      name: 'single chapter',
      entryId: 1,
      request: (title: LibraryTitle | undefined): AssignRequest => ({
        videos: [
          {
            file: 'Ted.Lasso.S02E01.mkv',
            action: 'place',
            title: {
              source: 'library',
              ...(title ? { libraryTitleId: title.id } : {}),
              kind: 'series',
            },
            season: 2,
            chapter: 1,
          },
        ],
        subtitles: [],
      }),
      expected:
        'TV Shows/Ted Lasso (2020) [tmdbid-97546]/Season 02/Ted Lasso (2020) - S02E01 - Goodbye Earl.mkv',
      title: 'Ted Lasso',
    },
    {
      name: 'multiple chapters',
      entryId: 1,
      request: (title: LibraryTitle | undefined): AssignRequest => ({
        videos: [
          {
            file: 'Ted.Lasso.S02E01.mkv',
            action: 'place',
            title: {
              source: 'library',
              ...(title ? { libraryTitleId: title.id } : {}),
              kind: 'series',
            },
            season: 2,
            chapter: 1,
            lastChapter: 2,
          },
        ],
        subtitles: [],
      }),
      expected:
        'TV Shows/Ted Lasso (2020) [tmdbid-97546]/Season 02/Ted Lasso (2020) - S02E01-E02 - Goodbye Earl + Lavender.mkv',
      title: 'Ted Lasso',
    },
    {
      name: 'subtitle flags and language',
      entryId: 3,
      request: (): AssignRequest => ({
        videos: [
          {
            file: 'Get.Out.2017.1080p.mkv',
            action: 'place',
            title: { source: 'free', kind: 'movie', title: 'Vacaciones', year: 2024 },
          },
        ],
        subtitles: [
          {
            file: 'Get.Out.2017.en.srt',
            video: 'Get.Out.2017.1080p.mkv',
            language: 'en',
            forced: true,
            sdh: true,
          },
        ],
      }),
      expected: 'Movies/Vacaciones (2024)/Vacaciones (2024).mkv',
      subtitleExpected: 'Movies/Vacaciones (2024)/Vacaciones (2024).en.forced.sdh.srt',
    },
    {
      name: 'free title has no id tag',
      entryId: 2,
      request: (): AssignRequest => ({
        videos: [
          {
            file: 'Mufasa.The.Lion.King.2024.2160p.mkv',
            action: 'place',
            title: { source: 'free', kind: 'movie', title: 'Mi título', year: 2025 },
          },
        ],
        subtitles: [],
      }),
      expected: 'Movies/Mi título (2025)/Mi título (2025).mkv',
    },
  ] as const

  it.each(cases)('plans $name with the library naming rules', async (testCase) => {
    const catalog = createCatalogDemo()
    const allTitles = (await catalog.libraryTitles()).titles
    const target =
      'title' in testCase ? allTitles.find((item) => item.discTitle === testCase.title) : undefined
    const detail = target ? await catalog.libraryTitle(target.id) : undefined
    const token = (await catalog.takeAssignLock(testCase.entryId)).token
    const request = testCase.request(detail)
    const plan = await catalog.planAssign(testCase.entryId, token, request)
    expect(plan.videos[0]?.finalPath).toBe(testCase.expected)
    if ('subtitleExpected' in testCase)
      expect(plan.subtitles[0]?.finalPath).toBe(testCase.subtitleExpected)
  })
})

describe('catalog demo localization', () => {
  afterEach(() => {
    document.documentElement.lang = 'es'
  })

  it('returns translated titles, synopses, genres, collections and episode titles', async () => {
    document.documentElement.lang = 'es'
    const catalog = createCatalogDemo()
    const spanish = await catalog.libraryTitles()
    const lion = spanish.titles.find((title) => title.tmdbId === 8587)
    expect(lion?.title).toBe('El rey león')
    expect(lion?.genres).toContain('Animación')
    expect(lion?.collection?.name).toBe('El rey león - Colección')
    const breakingBad = spanish.titles.find((title) => title.title === 'Breaking Bad')
    if (!breakingBad) throw new Error('Expected the seeded Breaking Bad title')
    const spanishDetail = await catalog.libraryTitle(breakingBad.id)
    expect(spanishDetail.overview).toContain('Walter White')
    expect(spanishDetail.seasons[0]?.episodes[0]?.title).toBe('Episodio 1')
    const originalVideoPath = spanishDetail.videos[0]?.path

    document.documentElement.lang = 'en'
    const english = await catalog.libraryTitles()
    const englishLion = english.titles.find((title) => title.tmdbId === 8587)
    expect(englishLion?.title).toBe('The Lion King')
    expect(englishLion?.genres).toContain('Animation')
    expect(englishLion?.collection?.name).toBe('The Lion King Collection')
    const englishBreakingBad = english.titles.find((title) => title.tmdbId === breakingBad.tmdbId)
    if (!englishBreakingBad) throw new Error('Expected the translated Breaking Bad title')
    const englishDetail = await catalog.libraryTitle(englishBreakingBad.id)
    expect(englishDetail.overview).toContain('Walter White')
    expect(englishDetail.seasons[0]?.episodes[0]?.title).toBe('Good Cop / Bad Cop')
    expect(englishDetail.videos[0]?.path).toBe(originalVideoPath)
  })
})
