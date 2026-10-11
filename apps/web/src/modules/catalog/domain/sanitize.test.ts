import vectors from '../../../../../../contracts/naming-cases.json'

import { entryName, FALLBACK_NAME, sanitizeName } from './sanitize'

interface SanitizeCase {
  name: string
  input: { name: string }
  expected: string
}

describe('sanitize vectors (contracts/naming-cases.json)', () => {
  it.each((vectors.sanitize.cases as SanitizeCase[]).map((c) => [c.name, c] as const))(
    'sanitize: %s',
    (_, c) => {
      expect(sanitizeName(c.input.name)).toBe(c.expected)
    },
  )

  it('uses the fallback name of the vectors', () => {
    expect(FALLBACK_NAME).toBe(vectors.sanitize.fallback)
  })

  it.each([
    ['a sanitized name is kept', 'Mission: Impossible', 'Mission - Impossible'],
    ['an empty name gets the fallback', '', vectors.sanitize.fallback],
    ['a name of signs gets the fallback', '?*|', vectors.sanitize.fallback],
    ['a name of dots gets the fallback', '..', vectors.sanitize.fallback],
  ])('entry name: %s', (_, raw, want) => {
    expect(entryName(raw)).toBe(want)
  })
})
