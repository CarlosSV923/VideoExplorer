import vectors from '../../../../../../contracts/naming-cases.json'

import { chapterTitle, discTitle, type ChapterNames } from './naming'

interface DiscTitleCase {
  name: string
  input: { original: string; english?: string }
  expected: string
}

interface ChapterTitleCase {
  name: string
  input: { chapters: { original?: string; english?: string }[] }
  expected: string
}

describe('title vectors (contracts/naming-cases.json)', () => {
  it.each((vectors.discTitle.cases as DiscTitleCase[]).map((c) => [c.name, c] as const))(
    'disc title: %s',
    (_, c) => {
      expect(discTitle(c.input.original, c.input.english ?? '')).toBe(c.expected)
    },
  )

  it.each((vectors.chapterTitle.cases as ChapterTitleCase[]).map((c) => [c.name, c] as const))(
    'chapter title: %s',
    (_, c) => {
      const chapters: ChapterNames[] = c.input.chapters.map((ch) => ({
        original: ch.original ?? '',
        english: ch.english ?? '',
      }))
      expect(chapterTitle(chapters)).toBe(c.expected)
    },
  )
})
