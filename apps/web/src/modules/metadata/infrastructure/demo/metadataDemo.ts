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

export function createMetadataDemo(catalogPorts?: CatalogPorts): MetadataPorts {
  return {
    searchTitles: async (query, year) => {
      const needle = query.trim().toLocaleLowerCase()
      const results = catalog.titles
        .filter(
          (item) =>
            (year === undefined || item.year === year) &&
            `${item.title.es} ${item.title.en} ${item.originalTitle}`
              .toLocaleLowerCase()
              .includes(needle),
        )
        .map((item) => ({
          source: 'tmdb' as const,
          tmdbId: item.tmdbId,
          kind: item.type as 'movie' | 'series',
          title: item.title.es,
          year: item.year,
          posterFile: `${String(item.tmdbId)}.svg`,
        }))
      const library = catalogPorts
        ? (await catalogPorts.libraryTitles()).titles
            .filter((item) =>
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
        : []
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
