import type { LibraryTitleSummary } from '../domain/types'

export function selectHeroTitles(
  titles: LibraryTitleSummary[],
  random: () => number = Math.random,
): LibraryTitleSummary[] {
  const candidates = titles.filter((title) => Boolean(title.backdrop))
  const count = Math.min(5, candidates.length)
  for (let index = 0; index < count; index += 1) {
    const swapWith = index + Math.floor(random() * (candidates.length - index))
    const current = candidates[index]
    const selected = candidates[swapWith]
    if (current && selected) {
      candidates[index] = selected
      candidates[swapWith] = current
    }
  }
  return candidates.slice(0, count)
}
