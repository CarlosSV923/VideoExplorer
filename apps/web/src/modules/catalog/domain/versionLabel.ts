/**
 * Version labels (spec RF-24, §5 <versión>): the label that tells apart the versions of one place,
 * read from the video and audio tracks. Same rules as internal/catalog/domain/version_label.go;
 * both pass the `versionLabel` section of contracts/version-labels.json.
 */

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

/**
 * The proposed label of one version. `incomplete` is true when a part that tells the versions
 * apart could not be named: the label leaves it out and the user completes it in the preview.
 */
export interface VersionLabel {
  text: string
  incomplete: boolean
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
 * share the resolution and the codec and their audio differs; " (2)", " (3)"… for equal labels
 * from the second on. A part that cannot be named and tells the versions apart is left out and
 * marks its version incomplete (see version_label.go).
 */
export function versionLabels(versions: readonly VersionMedia[]): VersionLabel[] {
  const resolutions = versions.map((v) => resolutionOf(v.width, v.height))
  const text: string[] = versions.map(() => '')
  const incomplete: boolean[] = versions.map((_, i) => resolutionAt(resolutions, i) === '')
  labelGroup(
    versions.map((_, i) => i),
    versions,
    resolutions,
    text,
    incomplete,
  )
  return withSuffixes(text, incomplete)
}

/** Labels the versions in members: by resolution, then by codec, then by audio. */
function labelGroup(
  members: readonly number[],
  versions: readonly VersionMedia[],
  resolutions: readonly string[],
  text: string[],
  incomplete: boolean[],
): void {
  const codecOf = (i: number): string => versionAt(versions, i).codec
  for (const sameResolution of partition(members, (i) => resolutionAt(resolutions, i))) {
    const byCodec = partition(sameResolution, codecOf)
    const codecsDiffer = byCodec.length > 1
    for (const sameCodec of byCodec) {
      const first = sameCodec[0] ?? 0
      let head = resolutionAt(resolutions, first)
      if (codecsDiffer) {
        const codec = CODEC_LABELS.get(versionAt(versions, first).codec)
        if (codec === undefined) for (const i of sameCodec) incomplete[i] = true
        else head = joinParts(head, codec)
      }
      if (sameCodec.length === 1) {
        text[first] = head
        continue
      }
      const tokens = sameCodec.map((i) => {
        const token = audioLabel(versionAt(versions, i).audio)
        if (token === undefined) {
          incomplete[i] = true
          return ''
        }
        return token
      })
      const differ = tokens.some((t) => t !== tokens[0])
      sameCodec.forEach((i, k) => {
        text[i] = differ ? joinParts(head, tokens[k] ?? '') : head
      })
    }
  }
}

/** Joins the parts of a label that are present, with a space. */
function joinParts(head: string, token: string): string {
  if (head === '') return token
  if (token === '') return head
  return `${head} ${token}`
}

/** The audio of one version: the language of its tracks, or Dual for two or more. Undefined when it has no name here. */
function audioLabel(tracks: readonly AudioMedia[]): string | undefined {
  if (tracks.length === 0) return undefined
  const names: string[] = []
  for (const track of tracks) {
    const name = languageOf(track)
    if (name === undefined) return undefined
    if (!names.includes(name)) names.push(name)
  }
  return names.length > 1 ? 'Dual' : names[0]
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

/** The label of a frame of width × height, or '' when it has no class. */
function resolutionOf(width: number, height: number): string {
  if (width <= 0 || height <= 0) return ''
  const reached = RESOLUTION_CLASSES.find(
    (c) => width * 100 >= c.width * RESOLUTION_REACH || height * 100 >= c.height * RESOLUTION_REACH,
  )
  return reached?.label ?? ''
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
function withSuffixes(text: readonly string[], incomplete: readonly boolean[]): VersionLabel[] {
  const count = new Map<string, number>()
  return text.map((label, i) => {
    const n = (count.get(label) ?? 0) + 1
    count.set(label, n)
    return { text: n > 1 ? `${label} (${String(n)})` : label, incomplete: incomplete[i] ?? false }
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
