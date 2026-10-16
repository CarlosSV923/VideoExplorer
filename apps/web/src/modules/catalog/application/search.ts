import type { LibraryTitleSummary } from '../domain/types'

export type Kind = 'movie' | 'series'
export type Sort = 'added' | 'title' | 'year'

export function normalizeSearch(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase()
    .trim()
}

export function matchesSearch(title: LibraryTitleSummary, query: string): boolean {
  const words = normalizeSearch(query).split(/\s+/).filter(Boolean)
  const haystack = normalizeSearch(
    [title.title, title.originalTitle, title.discTitle].filter(Boolean).join(' '),
  )
  return words.every((word) => haystack.includes(word))
}

export function sortTitles(
  titles: readonly LibraryTitleSummary[],
  sort: Sort,
): LibraryTitleSummary[] {
  return [...titles].sort((a, b) => {
    if (sort === 'added') return Date.parse(b.addedAt) - Date.parse(a.addedAt)
    if (sort === 'year') return b.year - a.year || a.title.localeCompare(b.title)
    return a.title.localeCompare(b.title, undefined, { sensitivity: 'base' })
  })
}
