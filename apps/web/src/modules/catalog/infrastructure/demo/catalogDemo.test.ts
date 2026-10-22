import { afterEach, describe, expect, it } from 'vitest'

import { createCatalogDemo } from './catalogDemo'

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
