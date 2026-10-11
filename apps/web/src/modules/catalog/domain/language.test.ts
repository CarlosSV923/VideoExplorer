import vectors from '../../../../../../contracts/naming-cases.json'

import { NameError } from './errors'
import { normalizeLanguage } from './language'

interface LanguageCase {
  name: string
  input: { language: string }
  expected: string | { error: string }
}

describe('subtitle language vectors (contracts/naming-cases.json)', () => {
  it.each((vectors.subtitleLanguage.cases as LanguageCase[]).map((c) => [c.name, c] as const))(
    'subtitle language: %s',
    (_, c) => {
      if (typeof c.expected === 'string') {
        expect(normalizeLanguage(c.input.language)).toBe(c.expected)
        return
      }
      let caught: unknown
      try {
        normalizeLanguage(c.input.language)
      } catch (err) {
        caught = err
      }
      expect(caught).toBeInstanceOf(NameError)
      expect((caught as NameError).field).toBe(c.expected.error)
    },
  )
})
