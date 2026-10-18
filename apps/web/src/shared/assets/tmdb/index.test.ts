import en from '../../i18n/en.json'
import es from '../../i18n/es.json'
import { tmdbLogos } from '.'
import longSvg from './tmdb-primary-long-blue.svg?raw'
import shortSvg from './tmdb-primary-short-blue.svg?raw'

/** Byte sizes of the official files; the SVGs are pure ASCII, so length is bytes. */
const officialSizes = { long: 2295, short: 2065 } as const

const svgDataPrefix = 'data:image/svg+xml,'

/** Vite inlines small SVG imports as data URIs: decode the bundled markup. */
function bundledMarkup(url: string): string {
  expect(url.startsWith(svgDataPrefix)).toBe(true)
  return decodeURIComponent(url.slice(svgDataPrefix.length))
}

describe('TMDB attribution logos (RNF-10)', () => {
  it.each([
    ['short', tmdbLogos.short, shortSvg],
    ['long', tmdbLogos.long, longSvg],
  ] as const)('bundles the official %s logo unmodified', (name, url, svg) => {
    expect(svg.length).toBe(officialSizes[name])
    expect(svg.startsWith('<svg ')).toBe(true)
    expect(svg.trimEnd().endsWith('</svg>')).toBe(true)

    const markup = bundledMarkup(url)
    const viewBox = /viewBox="([^"]+)"/.exec(svg)?.[1]
    expect(viewBox).toBeDefined()
    expect(markup).toContain(`viewBox='${viewBox ?? ''}'`)
    for (const [, d] of svg.matchAll(/ d="([^"]+)"/g)) {
      expect(markup).toContain(d ?? '')
    }
  })

  it.each([shortSvg, longSvg])('has no script, foreign content or external reference', (svg) => {
    expect(svg).not.toMatch(/<script|<foreignObject|<image|href="(?!#)|url\((?!#)/i)
  })

  it('shows the official English attribution', () => {
    expect(en.attribution.tmdb).toBe(
      'This product uses the TMDB API but is not endorsed or certified by TMDB.',
    )
  })

  it('keeps the required English attribution in Spanish', () => {
    expect(es.attribution.tmdb).toBe(
      'This product uses the TMDB API but is not endorsed or certified by TMDB.',
    )
  })
})
