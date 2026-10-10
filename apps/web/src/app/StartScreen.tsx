import { useTranslation } from 'react-i18next'

/**
 * Phase 0 placeholder: no design yet (phase 9). It only proves that the
 * composition root, i18n and the test and lint pipeline work end to end.
 */
export function StartScreen() {
  const { t } = useTranslation()
  return (
    <main>
      <h1>{t('app.name')}</h1>
      <p>{t('app.tagline')}</p>
    </main>
  )
}
