import { describe, expect, it } from 'vitest'

import type { LibraryTitleSummary } from '../domain/types'
import { selectHeroTitles } from './heroSelection'

const title = (id: number, backdrop = true): LibraryTitleSummary => ({
  id,
  kind: id % 2 ? 'movie' : 'series',
  title: `Title ${String(id)}`,
  discTitle: `Title ${String(id)}`,
  year: 2024,
  idTag: true,
  addedAt: '2026-10-01T00:00:00Z',
  ...(backdrop ? { backdrop: `/backdrop-${String(id)}.jpg` } : {}),
})

describe('selectHeroTitles', () => {
  it('selects five unique titles from the supplied recent rows', () => {
    const candidates = Array.from({ length: 20 }, (_, index) => title(index + 1))
    const selected = selectHeroTitles(candidates, () => 0)

    expect(selected).toHaveLength(5)
    expect(new Set(selected.map((item) => item.id)).size).toBe(5)
    expect(selected.every((item) => candidates.includes(item))).toBe(true)
  })

  it('filters titles without backdrops and selects fewer than five when needed', () => {
    const candidates = [title(1), title(2, false), title(3), title(4, false)]

    expect(selectHeroTitles(candidates, () => 0).map((item) => item.id)).toEqual([1, 3])
    expect(selectHeroTitles([title(5, false)])).toEqual([])
  })
})
