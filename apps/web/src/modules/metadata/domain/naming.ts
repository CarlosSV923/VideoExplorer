/**
 * Title choice for names (spec §5): the disc title `<T>` and the chapter title
 * `<título cap.>`. Same rules as internal/metadata/domain/naming.go; both pass
 * contracts/naming-cases.json. Sanitizing and folder names come in phase 5.
 */

const genericTitle = /^(?:episode|episodio|cap[ií]tulo|chapter)[ ]+\d+$/iu

const numberedSuffix = /^(.*?)(?:[ ]*\(\d+\)|,[ ]*(?:part|parte)[ ]+\d+|[ ]+\d+)$/iu

const letter = /\p{L}/u
const latinLetter = /\p{Script=Latin}/u

/** The TMDB titles of one chapter: original in the series' language, English in en-US. "" means absent. */
export interface ChapterNames {
  readonly original: string
  readonly english: string
}

/**
 * Disc title `<T>` of a movie or series: the original title when all its letters
 * are Latin, else the English title, else the original. A title with no letters
 * counts as Latin. Empty only when both titles are empty.
 */
export function discTitle(original: string, english: string): string {
  const o = original.trim()
  const e = english.trim()
  if (e === '' || (o !== '' && isLatin(o))) return o
  return e
}

/**
 * Chapter title `<título cap.>` of a file that holds the given chapters. Each chapter
 * takes its original title when that is Latin, else its English title; generic titles
 * count as none. Several titles join with " + ", unless they all share the same base
 * (the title without its numbered suffix), which is then the title. Empty when no
 * chapter has a title.
 */
export function chapterTitle(chapters: readonly ChapterNames[]): string {
  const titles = chapters.map(chapterTitleOf).filter((t) => t !== '')
  const [first, ...rest] = titles
  if (first === undefined) return ''
  if (rest.length === 0) return first
  const base = titleBase(first)
  if (rest.every((t) => titleBase(t) === base)) return base
  return titles.join(' + ')
}

function chapterTitleOf(chapter: ChapterNames): string {
  const original = keepTitle(chapter.original)
  if (original !== '' && isLatin(original)) return original
  return keepTitle(chapter.english)
}

/** Trims a TMDB title and drops it when it is empty or generic. */
function keepTitle(title: string): string {
  const trimmed = title.trim()
  return genericTitle.test(trimmed) ? '' : trimmed
}

/** The title without its numbered suffix. A title whose base would be empty keeps its own text. */
function titleBase(title: string): string {
  const base = numberedSuffix.exec(title)?.[1]
  return base === undefined || base.trim() === '' ? title : base
}

/** True when every letter of `s` is in the Latin script. Digits, signs and combining marks are not letters. */
function isLatin(s: string): boolean {
  for (const ch of s) {
    if (letter.test(ch) && !latinLetter.test(ch)) return false
  }
  return true
}
