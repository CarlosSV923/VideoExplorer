import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'

import { SegmentedControl } from '@/shared/ui'

import { changeLanguage, type Language } from '.'
import { useLanguage } from './hooks'

export function LanguageToggle() {
  const { i18n, t } = useTranslation()
  const language = useLanguage()
  const queryClient = useQueryClient()
  return (
    <SegmentedControl
      label={t('language.label')}
      value={language}
      onChange={(next: Language) => {
        void changeLanguage(i18n, next).then(() =>
          queryClient.invalidateQueries({ queryKey: ['library'] }),
        )
      }}
      options={[
        { value: 'es', label: t('language.es') },
        { value: 'en', label: t('language.en') },
      ]}
    />
  )
}
