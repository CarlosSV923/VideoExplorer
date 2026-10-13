/**
 * Version labels (spec RF-24, §5 <versión>): the label that tells apart the versions of one place,
 * read from the video and audio tracks. Same rules as internal/catalog/domain/version_label.go;
 * both pass the `versionLabel` section of contracts/version-labels.json.
 */

import { NameError } from './errors'

/** What a version label reads from a video: its main video track and its audio tracks (ffprobe). */
export interface VersionMedia {
  width: number
  height: number
  /** The codec name of the main video track, as ffprobe writes it ("hevc"). */
  codec: string
  audio: readonly AudioMedia[]
}

/** One audio track: its language tag and its title (ffprobe). */
export interface AudioMedia {
  language: string
  title: string
}

interface ResolutionClass {
  label: string
  width: number
  height: number
}

/** Tried from the largest; a video takes the first class whose frame it reaches (see RESOLUTION_REACH). */
const RESOLUTION_CLASSES: readonly ResolutionClass[] = [
  { label: '2160p', width: 3840, height: 2160 },
  { label: '1440p', width: 2560, height: 1440 },
  { label: '1080p', width: 1920, height: 1080 },
  { label: '720p', width: 1280, height: 720 },
  { label: '576p', width: 1024, height: 576 },
  { label: '480p', width: 854, height: 480 },
]

/** Share of a class's frame, in percent, that a width or a height must reach. */
const RESOLUTION_REACH = 98

/** Short names of the codecs (RF-24). */
const CODEC_LABELS = new Map<string, string>([
  ['hevc', 'HEVC'],
  ['h264', 'H.264'],
  ['av1', 'AV1'],
  ['vp9', 'VP9'],
  ['mpeg2video', 'MPEG-2'],
  ['vc1', 'VC-1'],
])

/** Names in Spanish of the audio languages that have one (RF-24). Spanish has its own rule. */
const LANGUAGE_LABELS = new Map<string, string>([
  ['eng', 'Inglés'],
  ['en', 'Inglés'],
  ['jpn', 'Japonés'],
  ['ja', 'Japonés'],
  ['fra', 'Francés'],
  ['fre', 'Francés'],
  ['fr', 'Francés'],
  ['deu', 'Alemán'],
  ['ger', 'Alemán'],
  ['de', 'Alemán'],
  ['ita', 'Italiano'],
  ['it', 'Italiano'],
  ['por', 'Portugués'],
  ['pt', 'Portugués'],
  ['kor', 'Coreano'],
  ['ko', 'Coreano'],
  ['zho', 'Chino'],
  ['chi', 'Chino'],
  ['zh', 'Chino'],
  ['rus', 'Ruso'],
  ['ru', 'Ruso'],
])

const SPANISH_LANGUAGES = new Set(['spa', 'es', 'esp'])

/** Words of a Spanish track's title that make it Castellano (RF-24). */
const CASTELLANO_TITLES = ['castellano', 'españa', 'espana', 'es-es']

/**
 * Returns the label of each version of one place, in the order given (RF-24): the resolution;
 * the codec when some versions share the resolution and their codecs differ; the audio when some
 * share the resolution and the codec and their audio differs; and " (2)", " (3)"… for equal labels
 * from the second on. A version whose label needs what the rule does not decide throws a
 * NameError: "video", "codec" or "audio" (see version_label.go).
 */
export function versionLabels(versions: readonly VersionMedia[]): string[] {
  const resolutions = versions.map((v) => {
    const label = resolutionOf(v.width, v.height)
    if (label === undefined) throw new NameError('video')
    return label
  })
  const labels: string[] = versions.map(() => '')
  labelGroup(
    versions.map((_, i) => i),
    versions,
    resolutions,
    labels,
  )
  return withSuffixes(labels)
}

/** Labels the versions in members: by resolution, then by codec, then by audio. */
function labelGroup(
  members: readonly number[],
  versions: readonly VersionMedia[],
  resolutions: readonly string[],
  labels: string[],
): void {
  const codecOf = (i: number): string => versionAt(versions, i).codec
  for (const sameResolution of partition(members, (i) => resolutionAt(resolutions, i))) {
    const byCodec = partition(sameResolution, codecOf)
    const codecsDiffer = byCodec.length > 1
    for (const sameCodec of byCodec) {
      const first = sameCodec[0] ?? 0
      let head = resolutionAt(resolutions, first)
      if (codecsDiffer) head += ` ${codecLabel(versionAt(versions, first))}`
      if (sameCodec.length === 1) {
        labels[first] = head
        continue
      }
      const { tokens, differ } = audioTokens(versions, sameCodec)
      sameCodec.forEach((i, k) => {
        labels[i] = differ ? `${head} ${tokens[k] ?? ''}` : head
      })
    }
  }
}

/** The audio label of each version in members, and whether they differ. */
function audioTokens(
  versions: readonly VersionMedia[],
  members: readonly number[],
): { tokens: string[]; differ: boolean } {
  const tokens = members.map((i) => audioLabel(versionAt(versions, i).audio))
  const differ = tokens.some((t) => t !== tokens[0])
  return { tokens, differ }
}

/** The audio of one version: the language of its tracks, or Dual for two or more. */
function audioLabel(tracks: readonly AudioMedia[]): string {
  if (tracks.length === 0) throw new NameError('audio')
  const names: string[] = []
  for (const track of tracks) {
    const name = languageOf(track)
    if (name === undefined) throw new NameError('audio')
    if (!names.includes(name)) names.push(name)
  }
  return names.length > 1 ? 'Dual' : (names[0] ?? '')
}

/** The name of one audio track's language, or undefined for a language that has no name here. */
function languageOf(track: AudioMedia): string | undefined {
  const lang = track.language.toLowerCase()
  if (SPANISH_LANGUAGES.has(lang) || lang.startsWith('es-')) {
    const title = track.title.toLowerCase()
    return CASTELLANO_TITLES.some((word) => title.includes(word)) ? 'Castellano' : 'Latino'
  }
  return LANGUAGE_LABELS.get(lang)
}

/** The short name of a version's codec, or a NameError for "codec". */
function codecLabel(version: VersionMedia): string {
  const label = CODEC_LABELS.get(version.codec)
  if (label === undefined) throw new NameError('codec')
  return label
}

/** The label of a frame of width × height, or undefined when it has no class. */
function resolutionOf(width: number, height: number): string | undefined {
  if (width <= 0 || height <= 0) return undefined
  const reached = RESOLUTION_CLASSES.find(
    (c) => width * 100 >= c.width * RESOLUTION_REACH || height * 100 >= c.height * RESOLUTION_REACH,
  )
  return reached?.label
}

/** Splits members into groups by key, in the order of their first member. */
function partition(members: readonly number[], key: (i: number) => string): number[][] {
  const groups = new Map<string, number[]>()
  for (const i of members) {
    const k = key(i)
    const group = groups.get(k)
    if (group === undefined) groups.set(k, [i])
    else group.push(i)
  }
  return [...groups.values()]
}

/** The first of equal labels keeps its label; the next ones get " (2)", " (3)"… */
function withSuffixes(labels: readonly string[]): string[] {
  const count = new Map<string, number>()
  return labels.map((label) => {
    const n = (count.get(label) ?? 0) + 1
    count.set(label, n)
    return n > 1 ? `${label} (${String(n)})` : label
  })
}

function versionAt(versions: readonly VersionMedia[], i: number): VersionMedia {
  const version = versions[i]
  if (version === undefined) throw new RangeError(`no version ${String(i)}`)
  return version
}

function resolutionAt(resolutions: readonly string[], i: number): string {
  return resolutions[i] ?? ''
}
