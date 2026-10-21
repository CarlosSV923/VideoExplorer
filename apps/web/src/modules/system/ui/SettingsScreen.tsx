import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'

import { useScans, useRunScans } from '@/modules/catalog/application/queries'
import { useLogout } from '@/modules/identity/application/queries'
import { useSettings } from '@/modules/mediaservers/application/queries'
import { LanguageToggle } from '@/shared/i18n/LanguageToggle'
import { useFormat } from '@/shared/i18n/hooks'
import { Button, PageHeader, StatusChip } from '@/shared/ui'
import { AppLink } from '@/shared/ui/AppLink'
import { LibraryLayout } from '@/app/LibraryLayout'
import { tmdbLogos } from '@/shared/assets/tmdb'

export function SettingsScreen() {
  const { t } = useTranslation()
  const format = useFormat()
  const navigate = useNavigate()
  const scans = useScans()
  const runScans = useRunScans()
  const settings = useSettings()
  const logout = useLogout()
  const jellyfin = settings.data?.jellyfin
  return (
    <LibraryLayout>
      <PageHeader
        title={t('library.settings')}
        actions={
          <nav aria-label={t('settings.sections')} className="flex gap-2">
            <AppLink
              href="/settings/general"
              aria-current="page"
              className="rounded-pill border border-accent px-3 py-2 text-body-sm font-bold no-underline"
            >
              {t('settings.general')}
            </AppLink>
            <AppLink
              href="/settings/trash"
              className="rounded-pill border border-control px-3 py-2 text-body-sm font-bold no-underline"
            >
              {t('settings.trash')}
            </AppLink>
          </nav>
        }
      />
      <div className="page-padding flex max-w-4xl flex-col gap-4 py-6">
        <section className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-line p-5">
          <h2 className="m-0 text-heading">{t('settings.language')}</h2>
          <LanguageToggle />
        </section>
        <section className="flex flex-col gap-4 rounded-lg border border-line p-5">
          <div>
            <h2 className="m-0 text-heading">{t('settings.scanning')}</h2>
            <p className="m-0 mt-1 text-body-sm text-ink-2">{t('settings.scanningBody')}</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {(scans.data?.scans ?? []).map((scan) => (
              <div key={scan.kind} className="rounded-md bg-surface-2 p-4">
                <strong>{t(`settings.scan.${scan.kind}` as never)}</strong>
                <p className="m-0 mt-2 text-body-sm text-ink-2">
                  {scan.running
                    ? t('settings.scan.running')
                    : scan.lastFinishedAt
                      ? t('settings.scan.last', { date: format.dateTime(scan.lastFinishedAt) })
                      : t('settings.scan.never')}
                </p>
              </div>
            ))}
          </div>
          <Button
            onClick={() => {
              runScans.mutate()
            }}
            loading={runScans.isPending}
          >
            {t('settings.scanNow')}
          </Button>
        </section>
        <section className="flex flex-col gap-3 rounded-lg border border-line p-5">
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="m-0 text-heading">{t('settings.jellyfin')}</h2>
            {jellyfin && (
              <StatusChip
                tone={
                  jellyfin.state === 'connected'
                    ? 'success'
                    : jellyfin.state === 'error'
                      ? 'danger'
                      : 'neutral'
                }
              >
                {t(`settings.jellyfinState.${jellyfin.state}` as never)}
              </StatusChip>
            )}
          </div>
          {jellyfin && jellyfin.state === 'unconfigured' && (
            <p className="m-0 text-body-sm text-ink-2">{t('settings.jellyfinUnconfigured')}</p>
          )}
          {jellyfin && jellyfin.state === 'connected' && (
            <>
              <p className="m-0 text-body-sm text-ink-2">
                {t(`settings.translation.${jellyfin.translationSource}` as never)}
              </p>
              <pre className="m-0 overflow-x-auto rounded-md bg-surface-2 p-3 font-mono text-caption">{`${jellyfin.translation.libraryPath}\n${jellyfin.translation.movies}\n${jellyfin.translation.series}`}</pre>
              {jellyfin.lastNotificationAt && (
                <p className="m-0 text-caption text-ink-2">
                  {t('settings.lastNotification', {
                    date: format.date(jellyfin.lastNotificationAt),
                  })}
                </p>
              )}
            </>
          )}
          {jellyfin && jellyfin.state === 'error' && (
            <>
              <p className="m-0 text-body-sm text-danger">
                {jellyfin.lastError
                  ? t(`settings.jellyfinError.${jellyfin.lastError.code}` as never)
                  : t('settings.jellyfinUnknown')}
              </p>
              {jellyfin.lastError?.message && (
                <pre className="m-0 overflow-x-auto rounded-md bg-surface-2 p-3 font-mono text-caption">
                  {jellyfin.lastError.message}
                </pre>
              )}
              {jellyfin.lastErrorAt && (
                <p className="m-0 text-caption text-ink-2">
                  {t('settings.lastError', { date: format.date(jellyfin.lastErrorAt) })}
                </p>
              )}
            </>
          )}
        </section>
        <section className="flex flex-col gap-2 rounded-lg border border-line p-5">
          <h2 className="m-0 text-heading">{t('settings.tmdb')}</h2>
          <img src={tmdbLogos.long} alt="TMDB" className="h-8 w-fit" />
          <p className="m-0 text-body-sm text-ink-2">{t('attribution.tmdb')}</p>
        </section>
        <section className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-line p-5">
          <div>
            <h2 className="m-0 text-heading">{t('settings.session')}</h2>
            <p className="m-0 mt-1 text-body-sm text-ink-2">{t('settings.sessionBody')}</p>
          </div>
          <Button
            onClick={() => {
              logout.mutate(undefined, {
                onSuccess: () => {
                  void navigate({ to: '/login', search: {} }).catch(() => undefined)
                },
              })
            }}
            loading={logout.isPending}
          >
            {t('settings.logout')}
          </Button>
        </section>
        <section className="rounded-lg border border-line p-5">
          <h2 className="m-0 text-heading">{t('settings.about')}</h2>
          <p className="m-0 mt-2 text-body-sm text-ink-2">{t('settings.aboutBody')}</p>
        </section>
      </div>
    </LibraryLayout>
  )
}
