import { useTranslation } from 'react-i18next'

import type { AssignSubtitle, SubtitleSuggestion } from '../domain/types'
import { Checkbox, Select, TextField } from '@/shared/ui'

const languages = [
  ['es-419', 'Español (Latinoamérica)'],
  ['es', 'Español'],
  ['en', 'English'],
  ['pt-BR', 'Português (Brasil)'],
  ['fr', 'Français'],
  ['ja', '日本語'],
] as const

export function SubtitleList({
  suggestions,
  values,
  onChange,
}: {
  suggestions: readonly SubtitleSuggestion[]
  values: readonly AssignSubtitle[]
  onChange: (values: AssignSubtitle[]) => void
}) {
  const { t } = useTranslation()
  if (!suggestions.length) return null
  return (
    <section className="flex flex-col gap-4 rounded-lg border border-line bg-surface p-5">
      <h2 className="m-0 text-heading font-display">{t('assign.subtitles.title')}</h2>
      {suggestions.map((suggestion, index) => {
        const value = values[index] ?? { file: suggestion.file, language: 'es-419' }
        return (
          <div
            key={suggestion.file}
            className="grid gap-3 rounded-md border border-line bg-surface-2 p-3 md:grid-cols-[minmax(0,1fr)_180px_auto_auto] md:items-end"
          >
            <TextField label={t('assign.subtitles.file')} value={value.file} readOnly />
            <Select
              label={t('assign.subtitles.language')}
              value={value.language}
              onChange={(event) => {
                update(index, { language: event.currentTarget.value })
              }}
            >
              {languages.map(([code, label]) => (
                <option key={code} value={code}>
                  {label}
                </option>
              ))}
            </Select>
            <Checkbox
              label={t('assign.subtitles.forced')}
              checked={value.forced ?? false}
              onChange={(forced) => {
                update(index, { forced })
              }}
            />
            <Checkbox
              label={t('assign.subtitles.sdh')}
              checked={value.sdh ?? false}
              onChange={(sdh) => {
                update(index, { sdh })
              }}
            />
          </div>
        )
        function update(position: number, patch: Partial<AssignSubtitle>) {
          onChange(
            values.map((item, itemIndex) =>
              itemIndex === position ? { ...item, ...patch } : item,
            ),
          )
        }
      })}
    </section>
  )
}
