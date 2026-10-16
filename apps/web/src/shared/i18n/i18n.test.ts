import { changeLanguage, createI18n, storedLanguage } from '.'
import { formatDate, formatDuration, formatSize } from './format'
import en from './en.json'
import es from './es.json'
import enRaw from './en.json?raw'
import esRaw from './es.json?raw'

/** Keys repeated inside one object: JSON.parse silently keeps the last. */
function duplicateKeys(raw: string): string[] {
  const stack: Set<string>[] = []
  const dupes: string[] = []
  for (const m of raw.matchAll(/"((?:[^"\\]|\\.)*)"\s*:|[{}]/g)) {
    if (m[0] === '{') stack.push(new Set())
    else if (m[0] === '}') stack.pop()
    else {
      const keys = stack[stack.length - 1]
      const key = m[1] ?? ''
      if (keys?.has(key)) dupes.push(key)
      keys?.add(key)
    }
  }
  return dupes
}

function keys(obj: object, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([k, v]) =>
    typeof v === 'object' && v !== null ? keys(v as object, `${prefix}${k}.`) : [`${prefix}${k}`],
  )
}

describe('i18n', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('has no repeated keys', () => {
    expect(duplicateKeys(esRaw)).toEqual([])
    expect(duplicateKeys(enRaw)).toEqual([])
  })

  it('has the same keys in every language', () => {
    expect(keys(en).sort()).toEqual(keys(es).sort())
  })

  it('defaults to Spanish and remembers the choice', async () => {
    expect(storedLanguage()).toBe('es')
    const i18n = createI18n()
    expect(i18n.t('app.tagline')).toBe('Tu biblioteca de películas y series en el NAS')
    await changeLanguage(i18n, 'en')
    expect(i18n.t('app.tagline')).toBe('Your movie and series library on the NAS')
    expect(document.documentElement.lang).toBe('en')
    expect(storedLanguage()).toBe('en')
  })

  it('ignores an unknown stored language', () => {
    localStorage.setItem('ve.lang', 'fr')
    expect(storedLanguage()).toBe('es')
  })

  it('formats sizes, dates and durations with the active locale', () => {
    const es = createI18n('es')
    const en = createI18n('en')
    expect(formatSize(16_900_000_000, 'es', es.t)).toBe('16,9 GB')
    expect(formatSize(16_900_000_000, 'en', en.t)).toBe('16.9 GB')
    expect(formatDate(new Date('2017-12-01T12:00:00Z'), 'es')).toContain('dic')
    expect(formatDuration(7_140, 'es', es.t)).toBe('1 h 59 min')
  })
})
