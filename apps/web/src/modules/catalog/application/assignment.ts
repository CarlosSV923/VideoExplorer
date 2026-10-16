import type { AssignPlan, AssignSubtitle, AssignVideo, SubtitleSuggestion } from '../domain/types'

export type SaveState =
  | { enabled: true }
  | { enabled: false; reason: 'noPreview' | 'invalid' | 'duplicate' | 'incompleteLabel' }

export function saveState(plan: AssignPlan | null): SaveState {
  if (!plan) return { enabled: false, reason: 'noPreview' }
  if (!plan.valid) {
    if (plan.errors.some((issue) => issue.field === 'duplicate'))
      return { enabled: false, reason: 'duplicate' }
    if (plan.errors.some((issue) => issue.field === 'version'))
      return { enabled: false, reason: 'incompleteLabel' }
    return { enabled: false, reason: 'invalid' }
  }
  return { enabled: true }
}

export function subtitlesByVideo(
  subtitles: readonly SubtitleSuggestion[],
): Map<string, SubtitleSuggestion[]> {
  const grouped = new Map<string, SubtitleSuggestion[]>()
  for (const subtitle of subtitles) {
    const key = subtitle.video ?? ''
    const current = grouped.get(key) ?? []
    current.push(subtitle)
    grouped.set(key, current)
  }
  return grouped
}

export function mergeTitleIntoVideos(
  videos: readonly AssignVideo[],
  title: AssignVideo['title'] | undefined,
): AssignVideo[] {
  return videos.map((video) => (video.action === 'keep' || !title ? video : { ...video, title }))
}

export function initialVideoRequest(
  videos: readonly {
    file: string
    reading: {
      type: 'movie' | 'series' | 'unknown'
      title?: string
      year?: number
      season?: number
      chapter?: number
      lastChapter?: number
      part?: number
    }
    match?: AssignVideo['title']
  }[],
): AssignVideo[] {
  return videos.map((video) => ({
    file: video.file,
    action: 'place',
    ...(video.match ? { title: video.match } : {}),
    ...(video.reading.part !== undefined ? { part: video.reading.part } : {}),
    ...(video.reading.season !== undefined ? { season: video.reading.season } : {}),
    ...(video.reading.chapter !== undefined ? { chapter: video.reading.chapter } : {}),
    ...(video.reading.lastChapter !== undefined ? { lastChapter: video.reading.lastChapter } : {}),
  }))
}

export function initialSubtitleRequest(subtitles: readonly SubtitleSuggestion[]): AssignSubtitle[] {
  return subtitles.map((subtitle) => ({
    file: subtitle.file,
    ...(subtitle.video ? { video: subtitle.video } : {}),
    language: subtitle.language || 'es-419',
    forced: subtitle.forced,
    sdh: subtitle.sdh,
  }))
}
