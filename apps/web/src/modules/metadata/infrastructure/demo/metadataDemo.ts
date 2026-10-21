/* eslint @typescript-eslint/require-await: off */
import catalog from './catalog.json'
import type { MetadataPorts } from '@/modules/metadata/application/ports'
import type { ImageKind } from '@/modules/metadata/domain/types'
import type { CatalogPorts } from '@/modules/catalog/application/ports'

export function demoImage(
  title: string,
  kind: ImageKind | 'backdrop' | 'thumb',
  path?: string,
): string {
  if (path) {
    const size =
      kind === 'poster' ? 'w500' : kind === 'backdrop' ? 'w1280' : kind === 'logo' ? 'w500' : 'w300'
    return `https://image.tmdb.org/t/p/${size}${path}`
  }
  let hue = 0
  for (const char of title) hue = (hue * 31 + (char.codePointAt(0) ?? 0)) % 360
  const width = kind === 'poster' ? 500 : kind === 'backdrop' ? 1280 : 300
  const height = kind === 'poster' ? 750 : kind === 'backdrop' ? 720 : 169
  const safe = title.replace(/[<&>]/g, '')
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${String(width)}" height="${String(height)}" viewBox="0 0 ${String(width)} ${String(height)}"><defs><linearGradient id="g" x2=".8" y2="1"><stop stop-color="hsl(${String(hue)} 45% 34%)"/><stop offset="1" stop-color="hsl(${String((hue + 42) % 360)} 42% 12%)"/></linearGradient></defs><rect width="100%" height="100%" fill="url(#g)"/><text x="8%" y="88%" fill="#f5f5f7" font-family="sans-serif" font-size="${String(kind === 'poster' ? 28 : 48)}" font-weight="700">${safe}</text></svg>`
  return `data:image/svg+xml,${encodeURIComponent(svg)}`
}

export const demoCatalog = catalog

type DemoTitle = (typeof catalog.titles)[number] & { imdbId?: string }

function idSearch(query: string): { tmdbId?: number; kind?: 'movie' | 'series'; imdbId?: string } {
  const text = query.trim()
  if (/^\d+$/.test(text)) return { tmdbId: Number(text) }
  if (/^tt\d+$/i.test(text)) return { imdbId: text.toLowerCase() }
  let urlText = text
  if (!urlText.includes('://') && /^(?:www\.)?(?:themoviedb\.org|imdb\.com)\//i.test(urlText)) {
    urlText = `https://${urlText}`
  }
  try {
    const url = new URL(urlText)
    const host = url.hostname.toLowerCase()
    const segments = url.pathname.split('/').filter(Boolean)
    if (host === 'themoviedb.org' || host === 'www.themoviedb.org') {
      for (let index = 0; index + 1 < segments.length; index += 1) {
        const kind = segments[index]
        const match = /^(\d+)(?:-|$)/.exec(segments[index + 1] ?? '')
        if ((kind === 'movie' || kind === 'tv') && match) {
          return { tmdbId: Number(match[1]), kind: kind === 'tv' ? 'series' : 'movie' }
        }
      }
    }
    if (host === 'imdb.com' || host === 'www.imdb.com') {
      const imdbId = segments.find((segment) => /^tt\d+$/i.test(segment))
      if (imdbId) return { imdbId: imdbId.toLowerCase() }
    }
  } catch {
    // Non-URL text continues through the title search.
  }
  return {}
}

export function createMetadataDemo(catalogPorts?: CatalogPorts): MetadataPorts {
  return {
    searchTitles: async (query, year) => {
      const needle = query.trim().toLocaleLowerCase()
      const id = idSearch(query)
      const libraryTitles = catalogPorts ? (await catalogPorts.libraryTitles()).titles : []
      const library = libraryTitles
        .filter(
          (item) =>
            id.tmdbId === undefined &&
            !id.imdbId &&
            `${item.title} ${item.originalTitle ?? ''}`.toLocaleLowerCase().includes(needle),
        )
        .map((item) => ({
          source: 'library' as const,
          libraryTitleId: item.id,
          ...(item.tmdbId === undefined ? {} : { tmdbId: item.tmdbId }),
          kind: item.kind,
          title: item.title,
          year: item.year,
          ...(item.poster ? { posterFile: `${String(item.tmdbId ?? item.id)}.svg` } : {}),
        }))
      const results = catalog.titles
        .filter(
          (item) =>
            (year === undefined || item.year === year) &&
            (id.tmdbId !== undefined
              ? item.tmdbId === id.tmdbId && (!id.kind || item.type === id.kind)
              : id.imdbId
                ? (item as DemoTitle).imdbId?.toLowerCase() === id.imdbId
                : `${item.title.es} ${item.title.en} ${item.originalTitle}`
                    .toLocaleLowerCase()
                    .includes(needle)),
        )
        .map((item) => {
          const libraryHit = libraryTitles.find(
            (hit) => hit.tmdbId === item.tmdbId && hit.kind === item.type,
          )
          return {
            source: libraryHit ? ('library' as const) : ('tmdb' as const),
            ...(libraryHit ? { libraryTitleId: libraryHit.id } : {}),
            tmdbId: item.tmdbId,
            kind: item.type as 'movie' | 'series',
            title: item.title.es,
            year: item.year,
            posterFile: `${String(item.tmdbId)}.svg`,
          }
        })
      return { tmdbAvailable: true, library, tmdb: results }
    },
    refreshTitleMetadata: async (titleId) => {
      if (!catalogPorts) return Promise.reject(new Error('Not available in demo'))
      return catalogPorts.libraryTitle(titleId)
    },
    loadTitleMetadata: async (titleId) => {
      if (!catalogPorts) return Promise.reject(new Error('Not available in demo'))
      return catalogPorts.libraryTitle(titleId)
    },
    imageUrl: (kind: ImageKind, file: string) => {
      const id = Number(file.replace(/\.svg$/, ''))
      const title =
        catalog.titles.find((item) => item.tmdbId === id)?.title.es ?? file.replace(/\.svg$/, '')
      const item = catalog.titles.find((candidate) => candidate.tmdbId === id)
      const path =
        kind === 'poster'
          ? item?.posterPath
          : kind === 'backdrop'
            ? item?.backdropPath
            : kind === 'logo'
              ? item?.logoPath
              : undefined
      return demoImage(title, kind, path)
    },
  }
}
