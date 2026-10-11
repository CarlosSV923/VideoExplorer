/**
 * Library paths of spec §5: the folder and the file of a movie or a chapter, and the file of a
 * subtitle. Same rules as internal/catalog/domain/paths.go; both pass the `paths` section of
 * contracts/naming-cases.json. The disc title and the chapter title are chosen by the caller
 * (metadata's discTitle and chapterTitle). Pure: no I/O.
 */

import { NameError } from './errors'
import { normalizeLanguage } from './language'
import { cleanChars, FALLBACK_NAME, finishName } from './sanitize'

/** Longest name of a file or a folder in the library, in UTF-8 bytes (spec §5). */
const MAX_NAME_BYTES = 255

const extensionPattern = /^\.[A-Za-z0-9]+$/

/** A movie or a series as its folder names it. */
export interface Title {
  /** The disc title <T>, already chosen by the caller. Empty is an error; a name that sanitizes to nothing becomes FALLBACK_NAME. */
  readonly name: string
  /** <A>: the year. 0 when unknown, and then the " (<A>)" part is left out. */
  readonly year: number
  /** The TMDB id; 0 for a free title, which has no tag. */
  readonly tmdbId: number
  /** Whether the tag [tmdbid-ID] is on for a linked title (RF-45). */
  readonly idTag: boolean
}

/** A movie file (spec §5, Movies/ rows). */
export interface MovieFile {
  /** MOVIES_DIR. */
  readonly root: string
  readonly title: Title
  /** The <versión> label of a second file of the same movie (RF-24); "" for none. */
  readonly version: string
  /** The number of a part (RF-22); 0 for none. A part and a version are exclusive. */
  readonly part: number
  /** The extension with its dot, in any letter case. */
  readonly extension: string
}

/** A chapter file, or one file with several consecutive chapters (spec §5, TV Shows/ rows). */
export interface ChapterFile {
  /** SERIES_DIR. */
  readonly root: string
  readonly title: Title
  /** The season; 0 holds the specials. */
  readonly season: number
  /** The first chapter of the file and the last one; last equals first for a single chapter. */
  readonly first: number
  readonly last: number
  /** The <título cap.> of the file's chapters, already chosen by the caller; "" when none. */
  readonly chapterTitle: string
  /** The <versión> label of a second file of the same chapter; "" for none. */
  readonly version: string
  readonly extension: string
}

/** A subtitle file that goes next to its video (spec §5, RF-23). */
export interface SubtitleFile {
  /** The <idioma> of the subtitle, as NormalizeLanguage writes it. */
  readonly language: string
  readonly forced: boolean
  readonly sdh: boolean
  readonly extension: string
}

/** Whether the tag [tmdbid-ID] is on when nothing else was chosen (RF-45): a linked title has it. */
export function defaultIdTag(tmdbId: number): boolean {
  return tmdbId > 0
}

/**
 * The path of a movie file (spec §5), relative to LIBRARY_PATH:
 * <root>/<T> (<A>)<ids>/<T> (<A>)<ids>[ - <versión> | - part<N>].<ext>.
 */
export function moviePath(m: MovieFile): string {
  const root = rootOf(m.root)
  const { head: rawHead, year, tag } = titleParts(m.title)
  const ext = extensionOf(m.extension)
  const suffix = movieSuffix(m.version, m.part)
  // The file is the longest name: the disc title is cut for it, so the folder keeps the same head.
  const head = shortenStem(
    rawHead,
    MAX_NAME_BYTES - byteLength(year + tag + suffix) - byteLength(ext),
  )
  const folder = fitHead(head, year + tag, '')
  const file = fitHead(head, year + tag + suffix, ext) + ext
  return joinPath(root, folder, file)
}

/**
 * The path of a chapter file (spec §5), relative to LIBRARY_PATH:
 * <root>/<T> (<A>)<ids>/Season <SS>/<T> (<A>) - S<SS>E<EE>[-E<FF>][ - <título cap.>][ - <versión>].<ext>.
 * The chapter file has no tag. When the name is longer than 255 bytes, the chapter title is cut
 * first and then the disc title.
 */
export function chapterPath(e: ChapterFile): string {
  const root = rootOf(e.root)
  const { head, year, tag } = titleParts(e.title)
  const ext = extensionOf(e.extension)
  const code = episodeCode(e.season, e.first, e.last)
  let versionSeg = ''
  if (e.version !== '') {
    const version = cleanChars(e.version)
    if (finishName(version) === '') throw new NameError('version')
    versionSeg = ' - ' + version
  }
  let chapter = cleanChars(e.chapterTitle)
  if (finishName(chapter) === '') chapter = ''
  const rest = year + ' - ' + code
  let chapterSeg = segment(chapter)
  if (
    byteLength(head) +
      byteLength(rest) +
      byteLength(chapterSeg) +
      byteLength(versionSeg) +
      byteLength(ext) >
      MAX_NAME_BYTES &&
    chapter !== ''
  ) {
    const room =
      MAX_NAME_BYTES -
      byteLength(head) -
      byteLength(rest) -
      byteLength(versionSeg) -
      byteLength(ext) -
      byteLength(' - ')
    chapterSeg = segment(shortenStem(chapter, room))
  }
  const folder = fitHead(head, year + tag, '')
  const file = fitHead(head, rest + chapterSeg + versionSeg, ext) + ext
  return joinPath(root, folder, seasonFolder(e.season), file)
}

/**
 * The path of a subtitle that goes with the video at videoPath (spec §5, RF-23): the same folder,
 * named after the video without its extension, then the language, the flags and the extension.
 */
export function subtitlePath(videoPath: string, s: SubtitleFile): string {
  if (videoPath === '') throw new NameError('video')
  const lang = normalizeLanguage(s.language)
  const ext = extensionOf(s.extension)
  let suffix = '.' + lang
  if (s.forced) suffix += '.forced'
  if (s.sdh) suffix += '.sdh'
  const slash = videoPath.lastIndexOf('/')
  const dir = videoPath.slice(0, slash + 1)
  const file = videoPath.slice(slash + 1)
  const dot = file.lastIndexOf('.')
  const stem = fitStem(dot >= 0 ? file.slice(0, dot) : file, suffix + ext)
  return joinPath(dir, stem + suffix + ext)
}

/** Checks a title and returns its disc title (steps 1 to 5), its year part and its tag part. */
function titleParts(t: Title): { head: string; year: string; tag: string } {
  if (t.name === '') throw new NameError('title')
  if (t.year < 0) throw new NameError('year')
  if (t.tmdbId < 0) throw new NameError('tmdbId')
  let head = cleanChars(t.name)
  if (finishName(head) === '') head = FALLBACK_NAME
  const year = t.year > 0 ? ' (' + String(t.year) + ')' : ''
  const tag = t.tmdbId > 0 && t.idTag ? ' [tmdbid-' + String(t.tmdbId) + ']' : ''
  return { head, year, tag }
}

/** The part of a movie file's name that tells it from the others: a version or a part number. */
function movieSuffix(version: string, part: number): string {
  if (part < 0) throw new NameError('part')
  if (part > 0 && version !== '') throw new NameError('part')
  if (part > 0) return ' - part' + String(part)
  if (version !== '') {
    const v = cleanChars(version)
    if (finishName(v) === '') throw new NameError('version')
    return ' - ' + v
  }
  return ''
}

/** The code S<SS>E<EE> or S<SS>E<EE>-E<FF> of a chapter file (spec §5), with two digits at least. */
function episodeCode(season: number, first: number, last: number): string {
  if (season < 0) throw new NameError('season')
  if (first < 1) throw new NameError('episode')
  if (last < first) throw new NameError('episodeEnd')
  let code = 'S' + pad2(season) + 'E' + pad2(first)
  if (last > first) code += '-E' + pad2(last)
  return code
}

/** The folder of a season: "Season 01", "Season 00" for the specials. */
function seasonFolder(season: number): string {
  return 'Season ' + pad2(season)
}

function rootOf(root: string): string {
  if (root === '') throw new NameError('root')
  return root
}

/** Checks a file's extension and returns it in lower case (spec §5, <ext>). */
function extensionOf(ext: string): string {
  if (!extensionPattern.test(ext)) throw new NameError('extension')
  return ext.toLowerCase()
}

/** The " - " segment of a name for a piece of text, or "" when the text is empty. */
function segment(text: string): string {
  return text === '' ? '' : ' - ' + text
}

/**
 * The stem of a name made of head and rest, shortened so that the stem and ext fit in 255 bytes:
 * the head is cut first, so the rest (the year, the tag, the codes, the version) stays.
 */
function fitHead(head: string, rest: string, ext: string): string {
  return fitStem(
    finishName(shortenStem(head, MAX_NAME_BYTES - byteLength(rest) - byteLength(ext)) + rest),
    ext,
  )
}

/** Shortens a stem so that the stem and ext fit in 255 bytes, at a character boundary. */
function fitStem(stem: string, ext: string): string {
  if (byteLength(stem) + byteLength(ext) <= MAX_NAME_BYTES) return stem
  return shortenStem(stem, MAX_NAME_BYTES - byteLength(ext))
}

/**
 * Cuts a stem to at most room bytes, at a character boundary, and drops the spaces and dots that
 * the cut leaves at its end.
 */
function shortenStem(stem: string, room: number): string {
  if (byteLength(stem) <= room) return stem
  let used = 0
  let out = ''
  for (const ch of stem) {
    const size = byteLength(ch)
    if (used + size > room) break
    used += size
    out += ch
  }
  return out.replace(/[ .]+$/, '')
}

/** Joins path parts with "/", as Go's path.Join does for names that carry no dot segments. */
function joinPath(...parts: string[]): string {
  return parts
    .filter((p) => p !== '')
    .join('/')
    .replace(/\/+/g, '/')
}

function byteLength(s: string): number {
  return new TextEncoder().encode(s).length
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}
