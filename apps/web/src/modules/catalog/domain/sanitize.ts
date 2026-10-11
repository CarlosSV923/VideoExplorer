/**
 * Sanitizing of spec §5 (steps 1 to 6) for one path component. Same rules as
 * internal/catalog/domain/naming.go; both pass the `sanitize` section of
 * contracts/naming-cases.json.
 */

/** Name of an unassigned entry whose name sanitizes to nothing (phase 4 decision, not in spec §5). */
export const FALLBACK_NAME = 'Sin nombre'

/** Unicode White_Space, the same set as Go's unicode.White_Space (JS \s also takes U+FEFF). */
const whiteSpace =
  /[\t\n\v\f\r \u{85}\u{a0}\u{1680}\u{2000}-\u{200a}\u{2028}\u{2029}\u{202f}\u{205f}\u{3000}]+/gu

/**
 * Applies the steps 1 to 6 of spec §5 to one path component. The result may be empty when
 * nothing is left of the name.
 */
export function sanitizeName(raw: string): string {
  let name = raw.normalize('NFC') // 1
  name = name.replace(/: /g, ' - ').replace(/:/g, '-') // 2
  name = name.replace(/[/\\]/g, '-') // 3
  name = name.replace(/"/g, "'") // 4
  name = name.replace(/[?*<>|\p{Cc}]/gu, '') // 5
  name = name.replace(whiteSpace, ' ') // 6
  return name.replace(/^ +/, '').replace(/[ .]+$/, '')
}

/** The name of an unassigned entry: sanitized, or FALLBACK_NAME when nothing is left of it. */
export function entryName(raw: string): string {
  const name = sanitizeName(raw)
  return name === '' ? FALLBACK_NAME : name
}
