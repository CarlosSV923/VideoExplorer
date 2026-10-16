import { useJobs } from '@/modules/ingestion/application/queries'
import { isJobActiveOrWaiting } from '@/modules/ingestion/application/uploadLogic'
import { Brand } from '@/shared/ui/Brand'
import { SearchField } from '@/shared/ui'
import { SearchIcon, UploadIcon } from '@/shared/ui/icons'
import { useLanguage } from '@/shared/i18n/hooks'
import { useUploadUi } from '@/modules/ingestion/ui/UploadUiContext'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'

export function Header() {
  const { t } = useTranslation()
  const language = useLanguage()
  const jobs = useJobs()
  const { openPicker, openPanel, busy } = useUploadUi()
  const active = (jobs.data ?? []).filter(isJobActiveOrWaiting).length + (busy ? 1 : 0)
  const links = [
    ['/peliculas', t('library.movies')],
    ['/series', t('library.seriesPlural')],
  ]
  return (
    <header className="sticky top-0 z-30 border-b border-line bg-bg/95 px-[var(--page-x,16px)] backdrop-blur">
      <div className="mx-auto flex min-h-16 max-w-screen-2xl items-center gap-4">
        <Brand />
        <nav aria-label={t('library.mainNavigation')} className="hidden items-center gap-5 md:flex">
          <a href="/" className="text-body-sm font-semibold no-underline">
            {t('library.home')}
          </a>
          {links.map(([href, label]) => (
            <a key={href} href={href} className="text-body-sm font-semibold no-underline">
              {label}
            </a>
          ))}
        </nav>
        <form
          action="/buscar"
          method="get"
          className="ml-auto hidden min-w-48 max-w-72 flex-1 md:block"
        >
          <SearchField
            label={t('library.search')}
            name="q"
            placeholder={t('library.searchPlaceholder')}
            shortcut
          />
        </form>
        <a
          href="/buscar"
          aria-label={t('library.search')}
          className="inline-flex size-control-sm items-center justify-center rounded-full md:hidden"
        >
          <SearchIcon />
        </a>
        <button
          type="button"
          aria-label={t('library.upload')}
          className="relative inline-flex min-h-control-sm items-center gap-2 rounded-pill border border-control px-3 text-body-sm font-bold"
          onClick={() => {
            openPicker()
            openPanel()
          }}
        >
          <UploadIcon />
          <span className="hidden lg:inline">{t('library.upload')}</span>
          {active > 0 && (
            <span
              aria-label={t('library.uploadsActive', { count: active })}
              className="size-2 rounded-full bg-accent"
            />
          )}
        </button>
        <a
          href="/ajustes/general"
          aria-label={t('library.settings')}
          className="inline-flex size-control-sm items-center justify-center rounded-full text-ink-2"
        >
          ⚙
        </a>
        <span className="sr-only">{language}</span>
      </div>
    </header>
  )
}

export function MobileNav() {
  const { t } = useTranslation()
  const links = [
    ['/', '⌂', t('library.home')],
    ['/peliculas', '▣', t('library.movies')],
    ['/series', '▤', t('library.seriesPlural')],
    ['/no-asignados', '□', t('library.unassigned')],
    ['/ajustes/general', '⚙', t('library.settings')],
  ]
  return (
    <nav
      aria-label={t('library.mobileNavigation')}
      className="fixed inset-x-0 bottom-0 z-30 grid grid-cols-5 border-t border-line bg-surface px-1 pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      {links.map(([href, icon, label]) => (
        <a
          key={href}
          href={href}
          className="flex min-h-16 flex-col items-center justify-center gap-0.5 text-caption text-ink-2 no-underline"
        >
          <span aria-hidden="true" className="text-lg">
            {icon}
          </span>
          {label}
        </a>
      ))}
    </nav>
  )
}

export function LibraryLayout({ children }: { children: ReactNode }) {
  return (
    <>
      <Header />
      <main className="mx-auto min-h-app max-w-screen-2xl page-padding pb-24 md:pb-10">
        {children}
      </main>
      <MobileNav />
    </>
  )
}
