import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'

import { useCatalogPorts } from '@/modules/catalog/application/ports'
import { LanguageToggle } from '@/shared/i18n/LanguageToggle'
import { Brand } from '@/shared/ui'
import { CatalogScreen } from '@/modules/catalog/ui/CatalogScreens'
import { RouteLoading } from './RouteLoading'

const TrashScreen = lazy(() =>
  import('@/modules/catalog/ui/TrashScreen').then((module) => ({ default: module.TrashScreen })),
)
const SettingsScreen = lazy(() =>
  import('@/modules/system/ui/SettingsScreen').then((module) => ({
    default: module.SettingsScreen,
  })),
)

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

export function MarkerScreen({
  screen,
  id,
  search,
}: {
  screen: ScreenKey
  id?: number
  search?: {
    sort?: string
    genre?: string
    incomplete?: string
    q?: string
    season?: number | undefined
  }
}) {
  const { t } = useTranslation()
  const catalog = useCatalogPorts()
  if (
    [
      'home',
      'movies',
      'series',
      'movieDetail',
      'seriesDetail',
      'edit',
      'collection',
      'search',
      'unassigned',
      'assign',
    ].includes(screen) &&
    typeof catalog.libraryTitles === 'function'
  ) {
    return <CatalogScreen screen={screen} id={id} search={search} />
  }
  if (screen === 'generalSettings')
    return (
      <Suspense fallback={<RouteLoading />}>
        <SettingsScreen />
      </Suspense>
    )
  if (screen === 'trashSettings')
    return (
      <Suspense fallback={<RouteLoading />}>
        <TrashScreen />
      </Suspense>
    )
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
