/* eslint @typescript-eslint/require-await: off */
import catalog from '@/modules/metadata/infrastructure/demo/catalog.json'
import type { CatalogPorts } from '@/modules/catalog/application/ports'
import type { AssignSuggestions, EditRequest, TitleHit } from '@/modules/catalog/domain/types'
import { chapterPath, moviePath, subtitlePath } from '@/modules/catalog/domain/paths'
import type { UploadJob } from '@/modules/ingestion/domain/types'
import type {
  LibrarySeason,
  LibraryTitle,
  LibraryTitles,
  LibraryVideo,
  TrashList,
  UnassignedSection,
} from '@/modules/catalog/domain/types'
import { demoImage, localizedDemoText } from '@/modules/metadata/infrastructure/demo/metadataDemo'
import { AppError } from '@/shared/kernel/errors'

type DemoTitle = (typeof catalog.titles)[number]
type DemoEpisode = LibrarySeason['episodes'][number]
const ids = new Map<number, number>()
const created = new Date('2026-10-17T12:00:00.000Z').toISOString()
let nextId = 1000
const seededMovieIds = new Set([
  8587, 11430, 272, 155, 1726, 10138, 68721, 438631, 27205, 129, 194, 496243, 597,
])
const episodeSelection: Record<string, (season: number, episode: number) => boolean> = {
  'breaking bad': (s, e) => !(s === 3 && [4, 5].includes(e)),
  dark: (s, e) => s === 1 || (s === 2 && e <= 4),
  chernobyl: () => true,
  'ted lasso': (s) => s === 1,
  'la casa de papel': (s) => s <= 2,
  'el juego del calamar': (s) => s === 1,
}
const seriesNames = Object.keys(episodeSelection)
const titles = catalog.titles.filter(
  (item) =>
    seededMovieIds.has(item.tmdbId) || seriesNames.includes(item.title.es.toLocaleLowerCase()),
)
const freeTitle = {
  id: 999,
  kind: 'movie' as const,
  title: 'Vacaciones en la playa',
  discTitle: 'Vacaciones en la playa',
  year: 2015,
  idTag: false,
  originalTitle: '',
  overview: '',
  runtime: 0,
  genres: [],
  seasons: [],
  videos: [
    {
      id: 9991,
      path: 'Vacaciones en la playa (2015).mkv',
      size: 800_000_000,
      version: '',
      part: 0,
      subtitles: [],
    },
  ],
}

function makeTitle(source: DemoTitle): LibraryTitle {
  const id = ids.get(source.tmdbId) ?? nextId++
  const name = localizedDemoText(source.title)
  const diskName = source.title.es
  const series = source.type === 'series'
  const present = episodeSelection[diskName.toLocaleLowerCase()]
  let videoId = id * 100
  const seasons: LibrarySeason[] = (source.seasons ?? []).map((season) => ({
    number: season.number,
    episodes: season.episodes.map((episode): DemoEpisode => {
      const hasFile = present?.(season.number, episode.number) ?? false
      const video = hasFile
        ? {
            id: videoId++,
            path: `${diskName} (${String(source.year)}) [tmdbid-${String(source.tmdbId)}]/Season ${String(season.number).padStart(2, '0')}/${diskName} - S${String(season.number).padStart(2, '0')}E${String(episode.number).padStart(2, '0')}.mkv`,
            size: 1_000_000_000,
            version: '',
            part: 0,
            season: season.number,
            chapter: episode.number,
            subtitles: [],
          }
        : undefined
      return {
        id: source.tmdbId * 100_000 + season.number * 100 + episode.number,
        number: episode.number,
        title: localizedDemoText(episode.title),
        overview: '',
        status: hasFile
          ? ('present' as const)
          : episode.airDate && episode.airDate > '2026-10-17'
            ? ('upcoming' as const)
            : ('missing' as const),
        files: video ? [video] : [],
        ...(episode.airDate ? { airDate: episode.airDate } : {}),
        thumbnail: demoImage(`${name}${String(episode.number)}`, 'still', episode.stillPath),
      }
    }),
  }))
  const videos: LibraryVideo[] = series
    ? seasons.flatMap((s) => s.episodes.flatMap((e) => e.files))
    : [
        {
          id: videoId++,
          path: `${source.discTitle} (${String(source.year)}) [tmdbid-${String(source.tmdbId)}]/${source.discTitle} (${String(source.year)}) [tmdbid-${String(source.tmdbId)}]${source.tmdbId === 155 ? ' - 2160p' : ''}.mkv`,
          size: 1_500_000_000,
          version: source.tmdbId === 155 ? '2160p' : '',
          part: 0,
          subtitles: [],
        },
      ]
  const firstVideo = videos[0]
  if (source.tmdbId === 438631 && firstVideo)
    videos.push({
      ...firstVideo,
      id: videoId++,
      path: 'Dune (2021) [tmdbid-438631]/Dune (2021) [tmdbid-438631] - 1080p Latino.mkv',
      size: 900_000_000,
      version: '1080p Latino',
    })
  if (source.tmdbId === 597 && firstVideo) {
    videos[0] = {
      ...firstVideo,
      part: 1,
      path: 'Titanic (1997) [tmdbid-597]/Titanic (1997) [tmdbid-597] - Part 1.mkv',
    }
    videos.push({
      ...firstVideo,
      id: videoId++,
      part: 2,
      path: 'Titanic (1997) [tmdbid-597]/Titanic (1997) [tmdbid-597] - Part 2.mkv',
    })
  }
  const collectionMovies = source.collection
    ? catalog.titles.filter(
        (item) => item.collection?.id === source.collection.id && item.type === 'movie',
      )
    : []
  return {
    id,
    kind: series ? 'series' : 'movie',
    title: name,
    discTitle: source.discTitle,
    year: source.year,
    tmdbId: source.tmdbId,
    idTag: true,
    originalTitle: source.originalTitle,
    overview: localizedDemoText(source.synopsis),
    runtime: source.runtime ?? 0,
    genres: source.genres.map((genre) => localizedDemoText(genre.name)),
    poster: demoImage(name, 'poster', source.posterPath),
    backdrop: demoImage(name, 'backdrop', source.backdropPath),
    ...(source.logoPath ? { logo: demoImage(name, 'logo', source.logoPath) } : {}),
    ...(source.collection
      ? {
          collection: {
            id: source.collection.id,
            name: localizedDemoText(source.collection.name),
            overview: '',
            movies: collectionMovies.map((movie) => {
              const libraryTitleId = ids.get(movie.tmdbId)
              return {
                id: movie.tmdbId,
                title: localizedDemoText(movie.title),
                originalTitle: movie.originalTitle,
                year: movie.year,
                poster: demoImage(localizedDemoText(movie.title), 'poster', movie.posterPath),
                inLibrary: libraryTitleId !== undefined,
                ...(libraryTitleId === undefined ? {} : { libraryTitleId }),
              }
            }),
          },
        }
      : {}),
    seasons,
    videos,
  }
}

export interface DemoCatalogAdapter extends CatalogPorts {
  addUploadEntry(name: string, paths: string[], size: number): void
  addFilteredExample(name: string, size: number): void
  addConsumedArchive(name: string, size: number): void
  finishSambaArchiveJob(id: string): void
}

export const demoSambaArchiveJob: UploadJob = {
  id: 'demo-samba-job-x7k2',
  batch: { id: 'demo-samba-batch-x7k2', name: 'x7k2', relation: 'none', createdAt: created },
  fileName: 'x7k2.part1.rar',
  relativePath: '',
  size: 80_000_000,
  received: 80_000_000,
  progress: 0,
  status: 'needs_password',
  createdAt: created,
  updatedAt: created,
}

export function createCatalogDemo(): DemoCatalogAdapter {
  Object.assign(demoSambaArchiveJob, {
    batch: { id: 'demo-samba-batch-x7k2', name: 'x7k2', relation: 'none', createdAt: created },
    fileName: 'x7k2.part1.rar',
    relativePath: '',
    size: 80_000_000,
    received: 80_000_000,
    progress: 0,
    status: 'needs_password',
    createdAt: created,
    updatedAt: created,
  })
  ids.clear()
  nextId = 1000
  for (const item of titles) if (!ids.has(item.tmdbId)) ids.set(item.tmdbId, nextId++)
  const library = titles.map(makeTitle)
  const localizeLibrary = () => {
    const language = document.documentElement.lang === 'en' ? 'en' : 'es'
    for (const title of library) {
      const source = catalog.titles.find((item) => item.tmdbId === title.tmdbId)
      if (!source) continue
      title.title = localizedDemoText(source.title, language)
      title.overview = localizedDemoText(source.synopsis, language)
      title.genres = source.genres.map((genre) => localizedDemoText(genre.name, language))
      for (const season of title.seasons) {
        const sourceSeason = source.seasons?.find((candidate) => candidate.number === season.number)
        for (const episode of season.episodes) {
          const sourceEpisode = sourceSeason?.episodes.find(
            (candidate) => candidate.number === episode.number,
          )
          if (sourceEpisode) episode.title = localizedDemoText(sourceEpisode.title, language)
        }
      }
      if (title.collection && source.collection) {
        title.collection.name = localizedDemoText(source.collection.name, language)
        for (const movie of title.collection.movies) {
          const sourceMovie = catalog.titles.find((candidate) => candidate.tmdbId === movie.id)
          if (sourceMovie) movie.title = localizedDemoText(sourceMovie.title, language)
        }
      }
    }
  }
  library.push(freeTitle)
  const initialEntries = [
    ['Ted.Lasso.S02.1080p.WEB', ['Ted.Lasso.S02E01.mkv', 'Ted.Lasso.S02E02.mkv']],
    ['Mufasa.The.Lion.King.2024.2160p.mkv', ['Mufasa.The.Lion.King.2024.2160p.mkv']],
    ['Get.Out.2017.1080p', ['Get.Out.2017.1080p.mkv', 'Get.Out.2017.en.srt']],
    ['The.Dark.Knight.2008.1080p.mkv', ['The.Dark.Knight.2008.1080p.mkv']],
    ['Copia de Samba', ['Copia.mkv']],
    ['Entrada vacía', []],
  ] as const
  const unassigned: UnassignedSection = {
    entries: initialEntries.map(([name, files], index) => ({
      id: index + 1,
      name,
      origin: 'samba',
      relation: index === 0 ? 'episodes' : 'none',
      loose: false,
      status: index === 5 ? 'copying' : 'ready',
      size: 1_000_000_000,
      files: files.map((path) => ({ path, size: 500_000_000, copying: index === 5 })),
    })),
    jobs: [demoSambaArchiveJob],
  }
  const trash: TrashList = {
    entries: [
      {
        id: 1,
        origin: 'library',
        reason: 'replaced',
        trashedAt: created,
        purgeAt: '2026-11-16T12:00:00.000Z',
        files: [{ name: 'Breaking Bad - S03E04.mkv', size: 900_000_000 }],
      },
      {
        id: 2,
        origin: 'upload',
        reason: 'extracted',
        trashedAt: created,
        purgeAt: '2026-11-16T12:00:00.000Z',
        relativePath: 'Inception.2010.part1.rar',
        files: [{ name: 'Inception.2010.part1.rar', size: 1_000_000_000 }],
      },
    ],
  }
  const scans = {
    scans: [
      { kind: 'unassigned' as const, running: false, lastFinishedAt: created },
      { kind: 'library' as const, running: false, lastFinishedAt: created },
    ],
  }
  const copyExample = unassigned.entries.find((entry) => entry.status === 'copying')
  if (copyExample) {
    window.setTimeout(() => {
      copyExample.status = 'ready'
      copyExample.files = copyExample.files.map((file) => ({ ...file, copying: false }))
    }, 2500)
  }
  const locks = new Map<number, string>()
  const seededMatchIds = new Map<number, number>([
    [1, 97546],
    [3, 419430],
    [4, 155],
  ])
  const titleHit = (id: number): TitleHit | undefined => {
    const source = catalog.titles.find((item) => item.tmdbId === id)
    if (!source) return undefined
    const existing = library.find((item) => item.tmdbId === id)
    return {
      source: existing ? 'library' : 'tmdb',
      ...(existing ? { libraryTitleId: existing.id } : { tmdbId: id }),
      kind: source.type as 'movie' | 'series',
      title: localizedDemoText(source.title),
      year: source.year,
      posterFile: `${String(id)}.svg`,
    }
  }
  const removeTitleVideos = (title: LibraryTitle, selected: LibraryVideo[]) => {
    const idsToRemove = new Set(selected.map((video) => video.id))
    title.videos = title.videos.filter((video) => !idsToRemove.has(video.id))
    for (const season of title.seasons) {
      for (const episode of season.episodes) {
        episode.files = episode.files.filter((video) => !idsToRemove.has(video.id))
        if (!episode.files.length && episode.status === 'present') episode.status = 'missing'
      }
    }
  }
  const suggestionsFor = (entryId: number): AssignSuggestions => {
    const entry = unassigned.entries.find((item) => item.id === entryId)
    if (!entry) throw new AppError('notFound', { status: 404 })
    const hintedId = seededMatchIds.get(entryId)
    const match = hintedId ? titleHit(hintedId) : undefined
    const videos = entry.files
      .filter((file) => !/\.(srt|ass|ssa|sub|idx)$/i.test(file.path))
      .map((file) => {
        const episode = file.path.match(/S(\d{2})E(\d{2})(?:E(\d{2}))?/i)
        const title = file.path
          .replace(/\.[^.]+$/, '')
          .replace(/[._]/g, ' ')
          .replace(/\b(19|20)\d{2}\b.*$/, '')
          .trim()
        return {
          file: file.path,
          reading: {
            type: episode ? ('series' as const) : ('movie' as const),
            title,
            ...(episode
              ? {
                  season: Number(episode[1]),
                  chapter: Number(episode[2]),
                  ...(episode[3] ? { lastChapter: Number(episode[3]) } : {}),
                }
              : {}),
          },
          ...(match ? { match } : {}),
        }
      })
    const subtitles = entry.files
      .filter((file) => /\.(srt|ass|ssa|sub|idx)$/i.test(file.path))
      .map((file) => ({
        file: file.path,
        language: /\.en(?:[-.]|\.)/i.test(file.path) ? 'en' : 'es-419',
        forced: /forced/i.test(file.path),
        sdh: /sdh/i.test(file.path),
        ...(videos[0] ? { video: videos[0].file } : {}),
      }))
    return {
      entryId,
      name: entry.name,
      relation: entry.relation,
      tmdbConfigured: true,
      ...(entry.relation === 'episodes' && match ? { title: match } : {}),
      videos,
      subtitles,
    }
  }
  const buildAssignPlan = (entryId: number, request: Parameters<CatalogPorts['planAssign']>[2]) => {
    const videos = request.videos.map((video) => {
      const targetTitle =
        video.title?.source === 'library'
          ? library.find((item) => item.id === video.title?.libraryTitleId)
          : undefined
      const target = targetTitle?.discTitle ?? video.title?.title
      const duplicate = targetTitle?.videos.find((item) =>
        targetTitle.kind === 'movie'
          ? item.part === (video.part ?? 0)
          : item.season === video.season && item.chapter === video.chapter,
      )
      const decision = video.duplicate
      const hasDuplicate = video.action === 'place' && duplicate !== undefined
      return {
        file: video.file,
        keep: video.action === 'keep' || decision === 'skip',
        duplicates: duplicate
          ? [
              {
                rel: duplicate.path,
                size: duplicate.size,
                ...(duplicate.version ? { version: duplicate.version } : {}),
              },
            ]
          : [],
        exactCopy: false,
        allowed: hasDuplicate ? ['replace' as const, 'keep_both' as const, 'skip' as const] : [],
        ...(decision ? { decision } : {}),
        ...(hasDuplicate && decision === 'keep_both'
          ? {
              version: video.version ?? '1080p',
              versionProposal: '1080p',
              versionIncomplete: /(?:^|[._ -])und(?:$|[._ -])/i.test(video.file) && !video.version,
            }
          : {}),
        ...(video.action === 'place' && video.title && (!hasDuplicate || decision)
          ? { finalPath: `${target ?? 'Título'}/${video.file}` }
          : {}),
      }
    })
    const errors = request.videos
      .filter((video) => video.action === 'place' && !video.title)
      .map((video) => ({ file: video.file, field: 'title', code: 'required' }))
    for (const [index, video] of request.videos.entries()) {
      const planned = videos[index]
      if (planned?.duplicates.length && !video.duplicate)
        errors.push({ file: video.file, field: 'duplicate', code: 'required' })
      if (planned?.versionIncomplete)
        errors.push({ file: video.file, field: 'version', code: 'incomplete' })
    }
    const subtitles = request.subtitles.map((subtitle) => {
      const video = videos.find((item) => item.file === subtitle.video)
      const finalPath = video?.finalPath
      return {
        file: subtitle.file,
        video: subtitle.video ?? '',
        ...(finalPath ? { finalPath: `${finalPath}.${subtitle.language}.srt` } : {}),
      }
    })
    return { entryId, valid: errors.length === 0, videos, subtitles, errors, warnings: [] }
  }
  const buildEditPlan = (title: LibraryTitle, request?: EditRequest) => {
    const choice = request?.title
    const source =
      choice?.source === 'tmdb'
        ? catalog.titles.find((item) => item.tmdbId === choice.tmdbId)
        : undefined
    const nextKind: 'movie' | 'series' =
      source?.type === 'series' || (!source && title.kind === 'series') ? 'series' : 'movie'
    const nextTitle = source?.discTitle ?? title.discTitle
    const nextYear = source?.year ?? title.year
    const nextId = source?.tmdbId ?? title.tmdbId ?? 0
    const videoPaths = new Map<number, string>()
    for (const video of title.videos) {
      const edit = request?.videos.find((item) => item.fileId === video.id)
      const extension = video.path.slice(video.path.lastIndexOf('.'))
      const path =
        nextKind === 'movie'
          ? moviePath({
              root: 'Movies',
              title: { name: nextTitle, year: nextYear, tmdbId: nextId, idTag: nextId > 0 },
              version: edit?.version ?? video.version,
              part: edit?.part ?? video.part,
              extension,
            })
          : chapterPath({
              root: 'TV Shows',
              title: { name: nextTitle, year: nextYear, tmdbId: nextId, idTag: nextId > 0 },
              season: edit?.season ?? video.season ?? 0,
              first: edit?.chapter ?? video.chapter ?? 1,
              last: edit?.lastChapter ?? video.lastChapter ?? video.chapter ?? 1,
              chapterTitle: edit?.chapterTitle ?? video.chapterTitle ?? '',
              version: edit?.version ?? video.version,
              extension,
            })
      videoPaths.set(video.id, path)
    }
    const firstPath = videoPaths.values().next().value
    const folder = firstPath
      ? firstPath
          .split('/')
          .slice(0, nextKind === 'series' ? -2 : -1)
          .join('/')
      : title.discTitle
    return {
      titleId: title.id,
      revision: 'demo',
      valid: true,
      kind: nextKind,
      title: nextTitle,
      folder,
      videos: title.videos.map((video) => {
        const finalPath = videoPaths.get(video.id) ?? video.path
        return {
          fileId: video.id,
          file: video.path,
          finalPath,
          changed: finalPath !== video.path,
          duplicates: [],
          exactCopy: false,
          allowed: [],
        }
      }),
      subtitles: title.videos.flatMap((video) =>
        video.subtitles.map((subtitle) => {
          const edit = request?.subtitles.find((item) => item.fileId === subtitle.id)
          const videoPath = videoPaths.get(video.id) ?? video.path
          const finalPath = subtitlePath(videoPath, {
            language: edit?.language ?? subtitle.language,
            forced: edit?.forced ?? subtitle.forced,
            sdh: edit?.sdh ?? subtitle.sdh,
            extension: subtitle.path.slice(subtitle.path.lastIndexOf('.')),
          })
          return {
            fileId: subtitle.id,
            file: subtitle.path,
            finalPath,
            changed: finalPath !== subtitle.path,
          }
        }),
      ),
      errors: [],
      warnings: [],
    }
  }
  const editResult = (title: LibraryTitle) => ({
    titleId: title.id,
    folder: title.discTitle,
    merged: false,
    changed: false,
  })
  const missing = () => Promise.reject(new AppError('notFound', { status: 404 }))
  return {
    finishSambaArchiveJob: (id) => {
      unassigned.jobs = unassigned.jobs.filter((job) => job.id !== id)
    },
    addFilteredExample: (name, size) => {
      trash.entries.push({
        id: Math.max(0, ...trash.entries.map((entry) => entry.id)) + 1,
        origin: 'upload',
        reason: 'filtered',
        trashedAt: new Date().toISOString(),
        purgeAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
        relativePath: name,
        files: [{ name, size }],
      })
    },
    addConsumedArchive: (name, size) => {
      trash.entries.push({
        id: Math.max(0, ...trash.entries.map((entry) => entry.id)) + 1,
        origin: 'upload',
        reason: 'extracted',
        trashedAt: new Date().toISOString(),
        purgeAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
        relativePath: name,
        files: [{ name, size }],
      })
    },
    addUploadEntry: (name, paths, size) => {
      const id = Math.max(0, ...unassigned.entries.map((entry) => entry.id)) + 1
      unassigned.entries.push({
        id,
        name,
        origin: 'upload',
        relation: paths.length > 1 ? 'episodes' : 'none',
        loose: false,
        status: 'ready',
        size,
        files: paths.map((path) => ({
          path,
          size: Math.floor(size / Math.max(1, paths.length)),
          copying: false,
        })),
      })
    },
    libraryTitles: async () => {
      localizeLibrary()
      return {
        titles: library.map((item) => ({
          id: item.id,
          kind: item.kind,
          title: item.title,
          discTitle: item.discTitle,
          year: item.year,
          ...(item.tmdbId === undefined ? {} : { tmdbId: item.tmdbId }),
          idTag: item.idTag,
          originalTitle: item.originalTitle,
          poster: item.poster,
          backdrop: item.backdrop,
          genres: item.genres,
          ...(item.collection
            ? { collection: { id: item.collection.id, name: item.collection.name } }
            : {}),
          ...(item.kind === 'series'
            ? {
                missingEpisodes: item.seasons
                  .flatMap((s) => s.episodes)
                  .filter((e) => e.status === 'missing').length,
              }
            : {}),
          addedAt: created,
        })),
      } as LibraryTitles
    },
    libraryTitle: async (id) => {
      localizeLibrary()
      return (
        library.find((item) => item.id === id) ??
        Promise.reject(new AppError('notFound', { status: 404 }))
      )
    },
    libraryCollection: async (id) => {
      const members = catalog.titles.filter(
        (item) => item.collection?.id === id && item.type === 'movie',
      )
      const source = members[0]?.collection
      if (!source) return missing()
      return {
        id,
        name: localizedDemoText(source.name),
        overview: '',
        poster: demoImage(localizedDemoText(source.name), 'poster', source.posterPath),
        backdrop: demoImage(
          localizedDemoText(source.name),
          'backdrop',
          members.find((item) => item.backdropPath)?.backdropPath,
        ),
        movies: members.map((movie) => {
          const libraryTitleId = ids.get(movie.tmdbId)
          return {
            id: movie.tmdbId,
            title: localizedDemoText(movie.title),
            originalTitle: movie.originalTitle,
            year: movie.year,
            poster: demoImage(localizedDemoText(movie.title), 'poster', movie.posterPath),
            inLibrary: libraryTitleId !== undefined,
            ...(libraryTitleId === undefined ? {} : { libraryTitleId }),
          }
        }),
      }
    },
    fileTechnical: async (_titleId, fileId) => ({
      path: library.flatMap((item) => item.videos).find((item) => item.id === fileId)?.path ?? '',
      size: 0,
      readable: true,
      duration: 0,
      video: { width: 0, height: 0, hdr: false, codec: '' },
      audio: [],
      subtitles: [],
    }),
    downloadTitleUrl: () => '#',
    downloadSeasonUrl: () => '#',
    downloadFileUrl: () => '#',
    planTitleEdit: async (id, request) => {
      const title = library.find((item) => item.id === id)
      return title ? buildEditPlan(title, request) : missing()
    },
    commitTitleEdit: async (id, request) => {
      const title = library.find((item) => item.id === id)
      if (!title) return missing()
      const oldTitle = title.discTitle
      const choice = request.title
      const source =
        choice.source === 'tmdb'
          ? catalog.titles.find((item) => item.tmdbId === choice.tmdbId)
          : undefined
      const preview = source ? buildEditPlan(title, request) : undefined
      if (source) {
        title.title = localizedDemoText(source.title)
        title.discTitle = source.discTitle
        title.originalTitle = source.originalTitle
        title.year = source.year
        title.tmdbId = source.tmdbId
        title.kind = source.type as 'movie' | 'series'
        title.idTag = true
      } else if (choice.source === 'free' && choice.title) {
        title.title = choice.title
        title.discTitle = choice.title
        title.year = choice.year ?? 0
        delete title.tmdbId
        title.idTag = false
      }
      for (const video of title.videos) {
        const planned = preview?.videos.find((item) => item.fileId === video.id)
        if (planned?.finalPath) video.path = planned.finalPath
        else video.path = video.path.replace(oldTitle, title.discTitle)
        for (const subtitle of video.subtitles) {
          const plannedSubtitle = preview?.subtitles.find((item) => item.fileId === subtitle.id)
          if (plannedSubtitle?.finalPath) subtitle.path = plannedSubtitle.finalPath
        }
      }
      return { ...editResult(title), changed: oldTitle !== title.discTitle }
    },
    planTitleIDTag: async (id) => {
      const title = library.find((item) => item.id === id)
      return title ? buildEditPlan(title) : missing()
    },
    commitTitleIDTag: async (id, request) => {
      const title = library.find((item) => item.id === id)
      if (!title) return missing()
      if (title.tmdbId) {
        title.idTag = request.idTag
        const tag = ` [tmdbid-${String(title.tmdbId)}]`
        for (const video of title.videos)
          video.path = request.idTag
            ? video.path.replace(title.discTitle, `${title.discTitle}${tag}`)
            : video.path.replace(tag, '')
      }
      return { ...editResult(title), changed: true }
    },
    moveTitleToUnassigned: async (titleId, scope) => {
      const title = library.find((item) => item.id === titleId)
      if (!title) return
      const selected = title.videos.filter(
        (video) =>
          scope.kind === 'title' ||
          (scope.kind === 'season' ? video.season === scope.season : video.id === scope.fileId),
      )
      if (!selected.length) return
      const files = selected.map((video) => ({
        path: video.path,
        size: video.size,
        copying: false,
      }))
      unassigned.entries.push({
        id: Math.max(0, ...unassigned.entries.map((entry) => entry.id)) + 1,
        name: title.title,
        origin: 'upload',
        relation: title.kind === 'series' ? 'episodes' : 'none',
        loose: false,
        status: 'ready',
        size: selected.reduce((total, video) => total + video.size, 0),
        files,
      })
      removeTitleVideos(title, selected)
      if (!title.videos.length) library.splice(library.indexOf(title), 1)
    },
    trashTitleScope: async (titleId, scope) => {
      const title = library.find((item) => item.id === titleId)
      if (!title) return
      const selected = title.videos.filter(
        (video) =>
          scope.kind === 'title' ||
          (scope.kind === 'season' ? video.season === scope.season : video.id === scope.fileId),
      )
      if (!selected.length) return
      trash.entries.push({
        id: Math.max(0, ...trash.entries.map((entry) => entry.id)) + 1,
        origin: 'library',
        reason: 'removed',
        trashedAt: new Date().toISOString(),
        purgeAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
        files: selected.map((video) => ({ name: video.path, size: video.size })),
      })
      removeTitleVideos(title, selected)
      if (!title.videos.length) library.splice(library.indexOf(title), 1)
    },
    unassigned: async () => {
      return {
        entries: unassigned.entries.map((entry) => ({
          ...entry,
          files: entry.files.map((file) => ({ ...file })),
        })),
        jobs: unassigned.jobs.map((job) => ({ ...job, batch: { ...job.batch } })),
      }
    },
    takeAssignLock: async (entryId) => {
      if (locks.has(entryId)) throw new Error('Assignment already active')
      const token = `demo-lock-${String(entryId)}`
      locks.set(entryId, token)
      return { token, expiresAt: new Date(Date.now() + 60_000).toISOString() }
    },
    releaseAssignLock: async (entryId, token) => {
      if (locks.get(entryId) === token) locks.delete(entryId)
    },
    assignSuggestions: async (entryId, token) => {
      if (locks.get(entryId) !== token) throw new Error('Assignment lock lost')
      return suggestionsFor(entryId)
    },
    planAssign: async (entryId, token, request) => {
      if (locks.get(entryId) !== token) throw new Error('Assignment lock lost')
      return buildAssignPlan(entryId, request)
    },
    commitAssign: async (entryId, token, request) => {
      if (locks.get(entryId) !== token) throw new Error('Assignment lock lost')
      const plan = buildAssignPlan(entryId, request)
      if (!plan.valid) throw new Error('Assignment choices are incomplete')
      const entryIndex = unassigned.entries.findIndex((entry) => entry.id === entryId)
      if (entryIndex < 0) throw new AppError('notFound', { status: 404 })
      const entry = unassigned.entries[entryIndex]
      if (!entry) throw new AppError('notFound', { status: 404 })
      const placed = plan.videos
        .filter((item) => !item.keep && item.finalPath)
        .map((item) => ({ file: item.file, path: item.finalPath ?? item.file }))
      for (const assignment of request.videos.filter(
        (video) => video.action === 'place' && video.title && video.duplicate !== 'skip',
      )) {
        const choice = assignment.title
        if (!choice) continue
        let target =
          choice.source === 'library'
            ? library.find((item) => item.id === choice.libraryTitleId)
            : undefined
        if (!target && choice.source === 'tmdb' && choice.tmdbId) {
          const source = catalog.titles.find((item) => item.tmdbId === choice.tmdbId)
          if (source) {
            if (!ids.has(source.tmdbId)) ids.set(source.tmdbId, nextId++)
            target = makeTitle(source)
            library.push(target)
          }
        }
        if (!target && choice.source === 'free' && choice.title) {
          target = {
            id: nextId++,
            kind: choice.kind ?? 'movie',
            title: choice.title,
            discTitle: choice.title,
            year: choice.year ?? 0,
            idTag: false,
            originalTitle: '',
            overview: '',
            runtime: 0,
            genres: [],
            seasons: [],
            videos: [],
          }
          library.push(target)
        }
        const planned = plan.videos.find((item) => item.file === assignment.file)
        if (!target || !planned?.finalPath) continue
        if (assignment.duplicate === 'replace' && planned.duplicates[0]) {
          const oldPath = planned.duplicates[0].rel
          const replaced = target.videos.find((item) => item.path === oldPath)
          target.videos = target.videos.filter((item) => item.path !== oldPath)
          for (const season of target.seasons) {
            for (const episode of season.episodes) {
              episode.files = episode.files.filter((item) => item.path !== oldPath)
              if (!episode.files.length) episode.status = 'missing'
            }
          }
          if (replaced) {
            trash.entries.push({
              id: Math.max(0, ...trash.entries.map((item) => item.id)) + 1,
              origin: 'library',
              reason: 'replaced',
              trashedAt: new Date().toISOString(),
              purgeAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
              files: [{ name: replaced.path, size: replaced.size }],
            })
          }
        }
        const video: LibraryVideo = {
          id: nextId++,
          path: planned.finalPath,
          size: entry.files.find((file) => file.path === assignment.file)?.size ?? 0,
          version: assignment.version ?? '',
          part: assignment.part ?? 0,
          subtitles: [],
          ...(assignment.season === undefined ? {} : { season: assignment.season }),
          ...(assignment.chapter === undefined ? {} : { chapter: assignment.chapter }),
          ...(assignment.lastChapter === undefined ? {} : { lastChapter: assignment.lastChapter }),
        }
        target.videos.push(video)
        if (
          target.kind === 'series' &&
          assignment.season !== undefined &&
          assignment.chapter !== undefined
        ) {
          let season = target.seasons.find((item) => item.number === assignment.season)
          if (!season) {
            season = { number: assignment.season, episodes: [] }
            target.seasons.push(season)
          }
          for (
            let number = assignment.chapter;
            number <= (assignment.lastChapter ?? assignment.chapter);
            number++
          ) {
            let episode = season.episodes.find((item) => item.number === number)
            if (!episode) {
              episode = {
                id: nextId++,
                number,
                title: `Episodio ${String(number)}`,
                overview: '',
                status: 'present',
                files: [],
              }
              season.episodes.push(episode)
            }
            episode.status = 'present'
            if (!episode.files.some((item) => item.id === video.id)) episode.files.push(video)
          }
        }
      }
      const keptFiles = entry.files.filter(
        (file) =>
          !request.videos.some((video) => video.file === file.path && video.action === 'place'),
      )
      if (keptFiles.length)
        unassigned.entries[entryIndex] = {
          ...entry,
          files: keptFiles,
          size: keptFiles.reduce((total, file) => total + file.size, 0),
        }
      else unassigned.entries.splice(entryIndex, 1)
      locks.delete(entryId)
      return { entryId, entryRemoved: keptFiles.length === 0, placed }
    },
    trashUnassignedEntry: async (entryId) => {
      const index = unassigned.entries.findIndex((entry) => entry.id === entryId)
      const entry = unassigned.entries[index]
      if (!entry || index < 0) return
      trash.entries.push({
        id: Math.max(0, ...trash.entries.map((item) => item.id)) + 1,
        origin: entry.origin,
        reason: 'removed',
        trashedAt: new Date().toISOString(),
        purgeAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
        relativePath: entry.name,
        files: entry.files.map((file) => ({ name: file.path, size: file.size })),
      })
      unassigned.entries.splice(index, 1)
    },
    deleteUnassignedEntry: async (entryId) => {
      const index = unassigned.entries.findIndex((entry) => entry.id === entryId)
      if (index >= 0) unassigned.entries.splice(index, 1)
    },
    trashUnassignedFile: async (entryId, request) => {
      const entry = unassigned.entries.find((item) => item.id === entryId)
      if (!entry) return
      const file = entry.files.find((item) => item.path === request.path)
      if (!file) return
      trash.entries.push({
        id: Math.max(0, ...trash.entries.map((item) => item.id)) + 1,
        origin: entry.origin,
        reason: 'removed',
        trashedAt: new Date().toISOString(),
        purgeAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
        relativePath: entry.name,
        files: [{ name: file.path, size: file.size }],
      })
      entry.files = entry.files.filter((item) => item.path !== request.path)
      entry.size = entry.files.reduce((total, item) => total + item.size, 0)
    },
    deleteUnassignedFile: async (entryId, request) => {
      const entry = unassigned.entries.find((item) => item.id === entryId)
      if (!entry) return
      entry.files = entry.files.filter((item) => item.path !== request.path)
      entry.size = entry.files.reduce((total, item) => total + item.size, 0)
      if (!entry.files.length)
        unassigned.entries = unassigned.entries.filter((item) => item.id !== entryId)
    },
    downloadUnassignedEntryUrl: () => '#',
    downloadUnassignedFileUrl: () => '#',
    scans: async () => scans,
    runScans: async () => ({
      scans: scans.scans.map((scan) => ({ ...scan, lastFinishedAt: new Date().toISOString() })),
    }),
    trash: async () => trash,
    emptyTrash: async () => {
      trash.entries.splice(0, trash.entries.length)
    },
    deleteTrashEntry: async (id) => {
      trash.entries = trash.entries.filter((entry) => entry.id !== id)
    },
    planTrashRestore: async (id) => {
      const entry = trash.entries.find((item) => item.id === id)
      if (!entry) return missing()
      return {
        entryId: id,
        files: entry.files.map((file) => ({
          file: file.name,
          destinations: [file.name],
          duplicates: [],
          allowed: ['replace', 'keep_both', 'cancel'],
          versionIncomplete: false,
        })),
      }
    },
    restoreTrashEntry: async (id) => {
      const entry = trash.entries.find((item) => item.id === id)
      if (!entry) return missing()
      const files = entry.files.map((file) => ({
        path: file.name,
        size: file.size,
        copying: false,
      }))
      unassigned.entries.push({
        id: Math.max(0, ...unassigned.entries.map((item) => item.id)) + 1,
        name: entry.relativePath ?? `Restaurado ${String(id)}`,
        origin: 'upload',
        relation: 'none',
        loose: false,
        status: 'ready',
        size: files.reduce((total, file) => total + file.size, 0),
        files,
      })
      trash.entries = trash.entries.filter((item) => item.id !== id)
      return { restored: files.map((file) => file.path) }
    },
  }
}
