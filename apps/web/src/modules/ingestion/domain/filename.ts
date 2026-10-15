/**
 * Filename analysis (spec RF-20, RF-22, RF-23, §5): what a file's name and its
 * folders say about it. Same rules as internal/ingestion/domain/filename.go; both
 * pass contracts/filename-cases.json. Pure: no I/O.
 */

/** Type of file: a video or an external subtitle (RF-07). */
export type MediaKind = 'video' | 'subtitle'

/** What a file is, as far as its name and folders say (RF-20). */
export type FileType = 'movie' | 'episode' | 'unknown'

/**
 * What the name of a file, and its folders, say about it. An absent key means the
 * name does not give it. `season` and `chapter` are 0 for specials (season 0) and
 * are present then. `forced` and `sdh` appear only when true.
 */
export interface FileAnalysis {
  readonly type: FileType
  readonly title?: string
  readonly year?: number
  readonly season?: number
  readonly chapter?: number
  readonly lastChapter?: number
  /** Movie part (cd1, part2, disc1, pt2). Movies only. */
  readonly part?: number
  /** Subtitle language: ISO 639-1, with a region only where Jellyfin recognizes it ("es-419"). */
  readonly language?: string
  readonly forced?: boolean
  readonly sdh?: boolean
  /** TMDB id of a tag [tmdbid-N] or {tmdb-N}, from the name or the nearest folder (RF-29). */
  readonly tmdbId?: number
  /** IMDb id of a tag [imdbid-ttN] (RF-29), found the same way. */
  readonly imdbId?: string
}

type Draft = { -readonly [K in keyof FileAnalysis]: FileAnalysis[K] }

interface EpisodeMark {
  readonly season: number
  readonly chapter: number
  /** 0 when the marker names one chapter. */
  readonly last: number
  readonly invalid: boolean
}

interface NameParts {
  readonly title: string
  /** 0 when the name has no year. */
  readonly year: number
  readonly episode: EpisodeMark | null
  /** The name is only a number: "01", "01 - Pilot". */
  readonly bare: number | null
  /** "Title - 001": absolute numbering (§8). */
  readonly anime: boolean
  /** 0 when the name has no part. */
  readonly part: number
}

interface SubtitleTags {
  /** '' when the name has no language. */
  readonly language: string
  readonly forced: boolean
  readonly sdh: boolean
}

interface Titled {
  readonly title: string
  readonly year: number
}

const VIDEO_EXTENSIONS = [
  'mkv',
  'mp4',
  'avi',
  'm4v',
  'ts',
  'm2ts',
  'mov',
  'wmv',
  'webm',
  'mpg',
  'mpeg',
]
const SUBTITLE_EXTENSIONS = ['srt', 'ass', 'ssa', 'sub', 'idx', 'vtt']

/** "Season 1", "Temporada 01", "S1". */
const SEASON_FOLDER = /^(?:season|temporada|s)[ ._-]*(\d{1,3})$/i
/** Season 0: "Specials" and "Especiales". */
const SPECIAL_FOLDER = /^(?:specials?|especiales)$/i

const EPISODE_SXXEYY = /^s(\d{1,3})e(\d{1,3})(?:-?e?(\d{1,3}))?$/i
const EPISODE_NXNN = /^(\d{1,2})x(\d{1,3})(?:-(\d{1,3}))?$/i
const PART = /^(?:cd|disc|part|pt)-?(\d{1,2})$/i
const YEAR = /^((?:19|20)\d{2})(?:-[A-Za-z][A-Za-z0-9]*)?$/
const BARE_NUMBER = /^\d{1,3}$/
const REGION = /^(?:[A-Za-z]{2}|\d{3})$/
const BRACKET_GROUP = /\[[^\]]*\]/g
const SEPARATORS = /[()._]/g
/** Id tags of a title (RF-29, §5): [tmdbid-N] (Jellyfin), {tmdb-N} and [imdbid-ttN]. */
const ID_TAG = /\[tmdbid-(\d+)\]|\{tmdb-(\d+)\}|\[imdbid-(tt\d+)\]/gi

/**
 * The technical tags that end a title: resolution, source, streaming service, codec,
 * HDR, bit depth, audio, with their scene group.
 */
const TAG =
  /^(?:\d{3,4}[pi]|[248]k|uhd|hdr(?:10)?\+?|\d{1,2}bit|bluray|blu-ray|bdrip|brrip|bd|dvdrip|dvdscr|hdrip|hdtv|webrip|web-dl|webdl|web|remux|hdcam|telesync|amzn|nf|dsnp|hmax|atvp|hulu|pcok|(?:x26[45]|h26[45]|hevc|avc|av1|vp9|xvid|divx)(?:-[a-z0-9]+)?|(?:aac|ac3|eac3|ddp?|dd|dts(?:-hd)?|truehd|atmos|flac|mp3|opus)\d*(?:-[a-z0-9]+)?)$/i

/** Regions Jellyfin recognizes, which the names keep (§5, spike 12.2). */
const KEPT_REGIONS = new Set(['pt-BR', 'pt-PT', 'fr-CA', 'zh-TW'])

/** Latin American regions, which turn es-XX into es-419 (§5). */
const LATIN_AMERICAN_REGIONS = new Set([
  'AR',
  'BO',
  'CL',
  'CO',
  'CR',
  'CU',
  'DO',
  'EC',
  'GT',
  'HN',
  'MX',
  'NI',
  'PA',
  'PE',
  'PR',
  'PY',
  'SV',
  'UY',
  'VE',
])

/** ISO 639-1 and 639-2 codes of the common languages, mapped to ISO 639-1. */
const ISO_CODES = new Map<string, string>([
  ['es', 'es'],
  ['spa', 'es'],
  ['en', 'en'],
  ['eng', 'en'],
  ['fr', 'fr'],
  ['fra', 'fr'],
  ['fre', 'fr'],
  ['de', 'de'],
  ['deu', 'de'],
  ['ger', 'de'],
  ['it', 'it'],
  ['ita', 'it'],
  ['pt', 'pt'],
  ['por', 'pt'],
  ['ja', 'ja'],
  ['jpn', 'ja'],
  ['ko', 'ko'],
  ['kor', 'ko'],
  ['zh', 'zh'],
  ['zho', 'zh'],
  ['chi', 'zh'],
  ['ru', 'ru'],
  ['rus', 'ru'],
  ['pl', 'pl'],
  ['pol', 'pl'],
  ['nl', 'nl'],
  ['nld', 'nl'],
  ['dut', 'nl'],
  ['sv', 'sv'],
  ['swe', 'sv'],
  ['da', 'da'],
  ['dan', 'da'],
  ['fi', 'fi'],
  ['fin', 'fi'],
  ['cs', 'cs'],
  ['ces', 'cs'],
  ['cze', 'cs'],
  ['hu', 'hu'],
  ['hun', 'hu'],
  ['tr', 'tr'],
  ['tur', 'tr'],
  ['el', 'el'],
  ['ell', 'el'],
  ['gre', 'el'],
  ['he', 'he'],
  ['heb', 'he'],
  ['ar', 'ar'],
  ['ara', 'ar'],
  ['hi', 'hi'],
  ['hin', 'hi'],
  ['th', 'th'],
  ['tha', 'th'],
  ['uk', 'uk'],
  ['ukr', 'uk'],
  ['ro', 'ro'],
  ['ron', 'ro'],
  ['rum', 'ro'],
  ['id', 'id'],
  ['ind', 'id'],
  ['vi', 'vi'],
  ['vie', 'vi'],
  ['no', 'no'],
  ['nor', 'no'],
])

/** Names of the common languages, in lowercase, mapped to ISO 639-1. */
const LANGUAGE_NAMES = new Map<string, string>([
  ['spanish', 'es'],
  ['español', 'es'],
  ['espanol', 'es'],
  ['castellano', 'es'],
  ['latino', 'es-419'],
  ['english', 'en'],
  ['inglés', 'en'],
  ['ingles', 'en'],
  ['french', 'fr'],
  ['français', 'fr'],
  ['francais', 'fr'],
  ['german', 'de'],
  ['deutsch', 'de'],
  ['alemán', 'de'],
  ['aleman', 'de'],
  ['italian', 'it'],
  ['italiano', 'it'],
  ['portuguese', 'pt'],
  ['português', 'pt'],
  ['japanese', 'ja'],
  ['japonés', 'ja'],
  ['japones', 'ja'],
  ['korean', 'ko'],
  ['coreano', 'ko'],
  ['chinese', 'zh'],
  ['chino', 'zh'],
  ['russian', 'ru'],
  ['ruso', 'ru'],
])

/** File names that say nothing about the title; a folder gives it (§5, RF-20). */
const GENERIC_NAMES = ['movie', 'film', 'video', 'main', 'feature', 'index']

const noSubtitleTags: SubtitleTags = { language: '', forced: false, sdh: false }

/** Analyzes a file from its path inside its entry (folders included, '/' or '\' separators). */
export function analyzeFile(relPath: string, kind: MediaKind): FileAnalysis {
  const { dirs, file } = splitRelPath(relPath)
  const base = trimExtension(file, kind)
  const cut = kind === 'subtitle' ? cutSubtitleTags(base) : { name: base, tags: noSubtitleTags }
  const name = parseName(cut.name)
  const season = findSeasonFolder(dirs)
  const folder = folderTitles(dirs, season)
  const picked = pickTitle(name, folder)

  const draft: Draft = { type: 'unknown' }
  const ids = idTagsOf(cut.name, dirs)
  if (ids.tmdbId !== 0) draft.tmdbId = ids.tmdbId
  if (ids.imdbId !== '') draft.imdbId = ids.imdbId
  if (name.episode !== null && name.episode.invalid) {
    setTitle(draft, picked)
  } else if (name.episode !== null) {
    draft.type = 'episode'
    draft.season = name.episode.season
    draft.chapter = name.episode.chapter
    if (name.episode.last > 0) draft.lastChapter = name.episode.last
    setTitle(draft, picked)
  } else if (season !== null) {
    // Inside a season folder the file name is a chapter title, unless it is a number.
    draft.type = 'episode'
    draft.season = season.season
    if (name.bare !== null) draft.chapter = name.bare
    setTitle(draft, folder)
  } else if (name.anime || name.bare !== null) {
    setTitle(draft, picked)
  } else {
    draft.type = 'movie'
    setTitle(draft, picked)
    if (name.part > 0) draft.part = name.part
  }
  if (draft.type === 'movie' && (draft.title ?? '') === '') draft.type = 'unknown'

  if (cut.tags.language !== '') draft.language = cut.tags.language
  if (cut.tags.forced) draft.forced = true
  if (cut.tags.sdh) draft.sdh = true
  return draft
}

/**
 * The TMDB id and the IMDb id that a file carries in its tags (RF-29): from its own name
 * first, else from its folders, the nearest first. A tag that does not read as an id is
 * ignored. 0 and '' when there is none.
 */
function idTagsOf(name: string, dirs: readonly string[]): { tmdbId: number; imdbId: string } {
  let tmdbId = 0
  let imdbId = ''
  for (const n of [name, ...[...dirs].reverse()]) {
    for (const m of n.matchAll(ID_TAG)) {
      const digits = m[1] ?? m[2] ?? ''
      if (tmdbId === 0 && digits !== '') {
        const id = Number(digits)
        if (id > 0 && Number.isSafeInteger(id)) tmdbId = id
      }
      if (imdbId === '' && m[3] !== undefined) imdbId = m[3].toLowerCase()
    }
    if (tmdbId !== 0 && imdbId !== '') break
  }
  return { tmdbId, imdbId }
}

function setTitle(draft: Draft, picked: Titled): void {
  if (picked.title !== '') draft.title = picked.title
  if (picked.year > 0) draft.year = picked.year
}

function splitRelPath(rel: string): { dirs: string[]; file: string } {
  const names = rel.split(/[\\/]/).filter((p) => p !== '' && p !== '.')
  const file = names.pop() ?? ''
  return { dirs: names, file }
}

function trimExtension(file: string, kind: MediaKind): string {
  const i = file.lastIndexOf('.')
  if (i <= 0) return file
  const exts = kind === 'subtitle' ? SUBTITLE_EXTENSIONS : VIDEO_EXTENSIONS
  return exts.includes(file.slice(i + 1).toLowerCase()) ? file.slice(0, i) : file
}

/**
 * Takes the tags off the end of a subtitle name without its extension (RF-23):
 * <name>.<language>[.forced|.sdh|.cc|.hi]. The language sits right before the flags.
 * The name keeps its first part.
 */
function cutSubtitleTags(name: string): { name: string; tags: SubtitleTags } {
  const parts = name.split('.')
  let j = parts.length - 1
  while (j >= 1 && isSubtitleFlag(parts, j)) j--
  const flags = parts.slice(j + 1)
  let language = ''
  if (j >= 1) {
    const code = languageOf(parts[j] ?? '', true)
    if (code !== null) {
      language = code
      j--
    }
  }
  let forced = false
  let sdh = false
  for (const f of flags) {
    switch (f.toLowerCase()) {
      case 'forced':
        forced = true
        break
      case 'sdh':
      case 'cc':
      case 'hi':
        sdh = true
        break
    }
  }
  return { name: parts.slice(0, j + 1).join('.'), tags: { language, forced, sdh } }
}

/**
 * Whether parts[j] is a flag: forced and sdh always; cc and hi only right after a
 * language (RF-23), so a lone hi is Hindi and a lone cc stays in the name.
 */
function isSubtitleFlag(parts: readonly string[], j: number): boolean {
  const seg = (parts[j] ?? '').toLowerCase()
  if (seg === 'forced' || seg === 'sdh') return true
  if (seg !== 'cc' && seg !== 'hi') return false
  return j >= 2 && languageOf(parts[j - 1] ?? '', true) !== null
}

/**
 * Reads a language tag: a name ("Spanish", "Español"), an ISO 639-1 or 639-2 code, or
 * one of them with a region ("es-MX"). With anyCase, codes in capitals count (subtitles);
 * in a video name a code must be lowercase, so a title word is not read as one.
 */
function languageOf(seg: string, anyCase: boolean): string | null {
  const dash = seg.indexOf('-')
  if (dash >= 0) {
    const code = ISO_CODES.get(seg.slice(0, dash).toLowerCase())
    const region = seg.slice(dash + 1)
    if (code === undefined || !REGION.test(region)) return null
    return regionalCode(code, region.toUpperCase())
  }
  const named = LANGUAGE_NAMES.get(seg.toLowerCase())
  if (named !== undefined) return named
  if (!anyCase && seg !== seg.toLowerCase()) return null
  return ISO_CODES.get(seg.toLowerCase()) ?? null
}

/**
 * The region rules of §5: es-MX and the other Latin American regions are es-419, es-ES
 * is es, en-* is en, the kept regions stay, and any other region is dropped.
 */
function regionalCode(code: string, region: string): string {
  if (code === 'es') {
    if (region === 'ES') return 'es'
    if (region === '419' || LATIN_AMERICAN_REGIONS.has(region)) return 'es-419'
    return 'es'
  }
  if (code === 'en') return 'en'
  const kept = `${code}-${region}`
  return KEPT_REGIONS.has(kept) ? kept : code
}

/**
 * Reads a name without its extension and subtitle tags: the title up to the first
 * marker, tag or year, then the year, the chapter or the part.
 */
function parseName(name: string): NameParts {
  const tokens = tokenize(name)
  const first = tokens[0] ?? ''
  if (tokens.length > 0 && BARE_NUMBER.test(first) && (tokens.length === 1 || tokens[1] === '-')) {
    return { title: '', year: 0, episode: null, bare: atoi(first), anime: false, part: 0 }
  }
  let end = tokens.length
  let episode: EpisodeMark | null = null
  let part = 0
  let anime = false
  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i] ?? ''
    const mark = episodeToken(tok)
    if (mark !== null) {
      episode = mark
      end = i
      break
    }
    if (i > 0 && TAG.test(tok)) {
      end = i
      break
    }
    const n = partToken(tok)
    if (n > 0) {
      part = n
      end = i
      break
    }
    if (i > 0 && tok === '-' && i + 1 < tokens.length && BARE_NUMBER.test(tokens[i + 1] ?? '')) {
      anime = true
      end = i
      break
    }
  }
  // The last year before the marker is the year: "Blade Runner 2049 (2017)" is 2017.
  let year = 0
  let yearAt = -1
  for (let i = 1; i < end; i++) {
    const y = yearToken(tokens[i] ?? '')
    if (y > 0) {
      year = y
      yearAt = i
    }
  }
  const words = tokens.slice(0, yearAt >= 0 ? yearAt : end)
  while (words.length > 0) {
    const last = words[words.length - 1] ?? ''
    if (last !== '-' && !isLanguageTag(last)) break
    words.pop()
  }
  return { title: words.join(' '), year, episode, bare: null, anime, part }
}

/**
 * Splits a name into words at dots, underscores and spaces. Brackets (site tags, TMDB
 * ids) go, parentheses separate, and "H.264" becomes one codec token.
 */
function tokenize(name: string): string[] {
  const words = name
    .replace(ID_TAG, ' ')
    .replace(BRACKET_GROUP, ' ')
    .replace(SEPARATORS, ' ')
    .split(/\s+/)
    .filter((w) => w !== '')
  const tokens: string[] = []
  for (let i = 0; i < words.length; i++) {
    let w = words[i] ?? ''
    const next = words[i + 1]
    if (w.toLowerCase() === 'h' && (next === '264' || next === '265')) {
      w = `H${next}`
      i++
    }
    tokens.push(w)
  }
  return tokens
}

function episodeToken(tok: string): EpisodeMark | null {
  const m = EPISODE_SXXEYY.exec(tok) ?? EPISODE_NXNN.exec(tok)
  if (m === null) return null
  const season = atoi(m[1] ?? '')
  const chapter = atoi(m[2] ?? '')
  const lastText = m[3] ?? ''
  if (lastText === '') return { season, chapter, last: 0, invalid: false }
  const last = atoi(lastText)
  return { season, chapter, last, invalid: last <= chapter }
}

/** The number of a movie part token (cd1, part2); 0 when the token is not one. */
function partToken(tok: string): number {
  const m = PART.exec(tok)
  if (m === null) return 0
  return atoi(m[1] ?? '')
}

/** The year of a token (1900 to 2099, with an optional scene group); 0 when it is not a year. */
function yearToken(tok: string): number {
  const m = YEAR.exec(tok)
  if (m === null) return 0
  return atoi(m[1] ?? '')
}

/** Whether a word at the end of a video title is a language tag, which the title leaves out. */
function isLanguageTag(word: string): boolean {
  return languageOf(word, false) !== null
}

function isGenericName(title: string): boolean {
  return GENERIC_NAMES.includes(title.toLowerCase())
}

/** The nearest season folder: its index in the path and its season (0 for specials). */
function findSeasonFolder(dirs: readonly string[]): { at: number; season: number } | null {
  for (let i = dirs.length - 1; i >= 0; i--) {
    const dir = dirs[i] ?? ''
    if (SPECIAL_FOLDER.test(dir)) return { at: i, season: 0 }
    const m = SEASON_FOLDER.exec(dir)
    if (m !== null) return { at: i, season: atoi(m[1] ?? '') }
  }
  return null
}

/**
 * The title and year that the folders give to a file: the folder above its season
 * folder when it is in one, else the file's own folder.
 */
function folderTitles(dirs: readonly string[], season: { at: number } | null): Titled {
  if (season !== null) {
    if (season.at === 0) return { title: '', year: 0 }
    return titledOf(dirs[season.at - 1] ?? '')
  }
  const last = dirs[dirs.length - 1]
  return last === undefined ? { title: '', year: 0 } : titledOf(last)
}

function titledOf(folderName: string): Titled {
  const n = parseName(folderName)
  return { title: n.title, year: n.year }
}

/**
 * The title and year of the file when it has its own title, else those of its folders.
 * A generic name with no year (movie.mkv) is no title of its own, so the folders decide,
 * and with no folder title there is none. The file's year wins; without one, the folder's
 * year comes only when the file's title is the folder's title (RF-20).
 */
function pickTitle(name: NameParts, folder: Titled): Titled {
  const generic = name.year === 0 && isGenericName(name.title)
  if (name.title === '' || generic) return folder
  const takesFolderYear =
    name.year === 0 && folder.title !== '' && sameTitle(name.title, folder.title)
  return { title: name.title, year: takesFolderYear ? folder.year : name.year }
}

/** Compares two titles ignoring case, dots, underscores and spaces (RF-20). */
function sameTitle(a: string, b: string): boolean {
  return normalizeTitle(a) === normalizeTitle(b)
}

/**
 * The form in which two titles are compared (RF-20): case, dots, underscores and spaces do not count,
 * and a dash that stands alone between spaces does not either. Only the comparison ignores it: the
 * title itself keeps the dash.
 */
function normalizeTitle(title: string): string {
  return title
    .toLowerCase()
    .replace(/[._]/g, ' ')
    .split(/\s+/)
    .filter((w) => w !== '' && w !== '-')
    .join(' ')
}

function atoi(digits: string): number {
  let n = 0
  for (const c of digits) n = n * 10 + (c.charCodeAt(0) - 48)
  return n
}
