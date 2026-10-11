/**
 * Language <idioma> of a subtitle file's name (spec §5). Same rules as
 * internal/catalog/domain/language.go; both pass the `subtitleLanguage` section of
 * contracts/naming-cases.json.
 */

import { NameError } from './errors'

/** ISO 639-1 with an optional region: "es", "es-MX" or "es-419". */
const languageTag = /^([a-z]{2})(?:-([a-z]{2}|\d{3}))?$/i

/** Regions Jellyfin 12.2 recognizes in a subtitle's language (spike 12.2). */
const jellyfinRegions = new Set(['pt-BR', 'pt-PT', 'fr-CA', 'zh-TW', 'es-419'])

/**
 * Spanish regions that Jellyfin does not know and that become es-419 (spec §5, decision 2026-10-10;
 * es-US added by the coordinator's decision, 2026-10-11).
 */
const latinAmericanSpanish = new Set([
  'es-AR',
  'es-BO',
  'es-CL',
  'es-CO',
  'es-CR',
  'es-CU',
  'es-DO',
  'es-EC',
  'es-SV',
  'es-GT',
  'es-HN',
  'es-MX',
  'es-NI',
  'es-PA',
  'es-PY',
  'es-PE',
  'es-PR',
  'es-UY',
  'es-US',
  'es-VE',
])

/**
 * Returns the language of a subtitle as its file name writes it (spec §5): ISO 639-1 with a
 * region only when Jellyfin recognizes it. Latin American Spanish regions become es-419; any
 * other region is dropped. Throws a NameError for "language" when the tag is not usable.
 */
export function normalizeLanguage(tag: string): string {
  const match = languageTag.exec(tag)
  if (match === null) throw new NameError('language')
  const [, primary = '', region] = match
  const lang = primary.toLowerCase()
  if (region === undefined) return lang
  const full = `${lang}-${region.toUpperCase()}`
  if (jellyfinRegions.has(full)) return full
  if (latinAmericanSpanish.has(full)) return 'es-419'
  return lang
}
