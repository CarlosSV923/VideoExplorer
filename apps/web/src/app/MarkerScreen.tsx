import { useTranslation } from 'react-i18next'

import { LanguageToggle } from '@/shared/i18n/LanguageToggle'
import { Brand } from '@/shared/ui'

export type ScreenKey =
  | 'home'
  | 'movies'
  | 'series'
  | 'movieDetail'
  | 'seriesDetail'
  | 'edit'
  | 'collection'
  | 'search'
  | 'unassigned'
  | 'assign'
  | 'generalSettings'
  | 'trashSettings'

export function MarkerScreen({ screen }: { screen: ScreenKey }) {
  const { t } = useTranslation()
  return (
    <main className="mx-auto flex min-h-app max-w-screen-xl flex-col gap-8 p-6">
      <header className="flex items-center justify-between gap-4">
        <Brand />
        <LanguageToggle />
      </header>
      <section className="flex min-h-80 flex-col justify-center gap-3 rounded-xl border border-line p-8">
        <h1 className="m-0 text-display font-bold text-ink-1">{t(`screens.${screen}.title`)}</h1>
        <p className="m-0 max-w-[60ch] text-body text-ink-2">{t(`screens.${screen}.body`)}</p>
      </section>
    </main>
  )
}
