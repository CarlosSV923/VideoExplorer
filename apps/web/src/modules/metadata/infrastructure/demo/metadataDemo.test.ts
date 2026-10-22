import { afterEach, describe, expect, it } from 'vitest'

import { createMetadataDemo, localizedDemoText } from './metadataDemo'

describe('metadata demo localization', () => {
  afterEach(() => {
    document.documentElement.lang = 'es'
  })

  it.each([
    { language: 'es', expected: 'El rey león' },
    { language: 'en', expected: 'The Lion King' },
  ] as const)('searches demo titles in $language', async ({ language, expected }) => {
    document.documentElement.lang = language
    const result = await createMetadataDemo().searchTitles('lion')
    expect(result.tmdb[0]?.title).toBe(expected)
  })

  it('falls back to the available catalog language', () => {
    expect(localizedDemoText({ es: 'Drama' }, 'en')).toBe('Drama')
    expect(localizedDemoText({ en: 'Animation' }, 'es')).toBe('Animation')
  })
})
