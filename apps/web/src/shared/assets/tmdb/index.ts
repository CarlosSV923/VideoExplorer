import tmdbShortLogo from './tmdb-primary-short-blue.svg'
import tmdbLongLogo from './tmdb-primary-long-blue.svg'

/**
 * Official TMDB logos for the attribution required by RNF-10. They are bundled
 * (the browser never asks TMDB for them) and are not modified; see README.md.
 */
export const tmdbLogos = {
  short: tmdbShortLogo,
  long: tmdbLongLogo,
} as const
