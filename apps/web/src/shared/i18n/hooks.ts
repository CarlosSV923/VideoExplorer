import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { defaultLanguage, isLanguage, type Language } from '.'
import { formatDate, formatDuration, formatRelative, formatSize } from './format'

export function useLanguage(): Language {
  const { i18n } = useTranslation()
  return isLanguage(i18n.language) ? i18n.language : defaultLanguage
}

export function useFormat() {
  const { t } = useTranslation()
  const language = useLanguage()
  return useMemo(
    () => ({
      size: (bytes: number) => formatSize(bytes, language, t),
      date: (date: Date | string) =>
        formatDate(typeof date === 'string' ? new Date(date) : date, language),
      duration: (seconds: number) => formatDuration(seconds, language, t),
      relative: (date: Date | string, now: number) =>
        formatRelative(typeof date === 'string' ? new Date(date) : date, now, language),
      list: (items: readonly string[]) =>
        new Intl.ListFormat(language, { type: 'disjunction' }).format(items),
    }),
    [language, t],
  )
}
