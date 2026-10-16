import type { TFunction } from 'i18next'

import type { Language } from '.'

const units = ['bytes', 'kb', 'mb', 'gb', 'tb'] as const

export function formatSize(bytes: number, language: Language, t: TFunction): string {
  let value = Math.max(0, bytes)
  let unit = 0
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000
    unit += 1
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1
  const number = new Intl.NumberFormat(language, {
    maximumFractionDigits: digits,
    minimumFractionDigits: digits,
  }).format(value)
  return t(`size.${units[unit] ?? 'tb'}`, { value: number })
}

export function formatDate(date: Date, language: Language) {
  return new Intl.DateTimeFormat(language, { day: 'numeric', month: 'short', year: 'numeric' })
    .format(date)
    .replace(/\./g, '')
}

export function formatDuration(seconds: number, language: Language, t: TFunction) {
  const totalMinutes = Math.max(0, Math.round(seconds / 60))
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  if (hours === 0) return t('duration.minutes', { count: minutes })
  if (minutes === 0) return t('duration.hours', { count: hours })
  return t('duration.hoursMinutes', { hours, minutes, locale: language })
}

const relativeSteps: [Intl.RelativeTimeFormatUnit, number][] = [
  ['second', 60],
  ['minute', 60],
  ['hour', 24],
  ['day', 30],
  ['month', 12],
  ['year', Infinity],
]
export function formatRelative(date: Date, now: number, language: Language) {
  let value = (date.getTime() - now) / 1000
  let unit: Intl.RelativeTimeFormatUnit = 'second'
  for (const [step, size] of relativeSteps) {
    unit = step
    if (Math.abs(value) < size) break
    value /= size
  }
  if (unit === 'second') {
    value = 0
    unit = 'minute'
  }
  return new Intl.RelativeTimeFormat(language, { numeric: 'auto', style: 'short' }).format(
    Math.round(value),
    unit,
  )
}
