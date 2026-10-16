import { describe, expect, it } from 'vitest'

import { matchesSearch, normalizeSearch, sortTitles } from './search'
import type { LibraryTitleSummary } from '../domain/types'

const summary = (overrides: Partial<LibraryTitleSummary> = {}): LibraryTitleSummary => ({
  id: 1,
  kind: 'movie',
  title: 'Película Ámbar',
  discTitle: 'Pelicula Ambar',
  year: 2024,
  idTag: true,
  addedAt: '2026-10-01T00:00:00Z',
  ...overrides,
})

describe('catalog search helpers', () => {
  it('normalizes accents, whitespace and case', () => {
    expect(normalizeSearch('  Película Ámbar  ')).toBe('pelicula ambar')
  })

  it('matches all query words against title aliases', () => {
    expect(matchesSearch(summary(), 'pelicula AMBAR')).toBe(true)
    expect(matchesSearch(summary(), 'película ciencia')).toBe(false)
  })

  it.each([
    ['added', [3, 2]],
    ['title', [3, 2]],
    ['year', [2, 3]],
  ] as const)('sorts by %s', (sort, expected) => {
    const titles = [
      summary({ id: 2, title: 'B', year: 2023, addedAt: '2026-10-02T00:00:00Z' }),
      summary({ id: 3, title: 'A', year: 2022, addedAt: '2026-10-03T00:00:00Z' }),
    ]
    expect(sortTitles(titles, sort).map((title) => title.id)).toEqual(expected)
  })
})
