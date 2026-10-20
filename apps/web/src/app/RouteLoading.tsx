import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

export function RouteLoading() {
  const { t } = useTranslation()
  const [showSkeleton, setShowSkeleton] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setShowSkeleton(true)
    }, 300)
    return () => {
      window.clearTimeout(timer)
    }
  }, [])

  return (
    <main
      className="min-h-app px-[var(--page-x,16px)] py-8"
      aria-busy="true"
      aria-label={t('common.loading')}
    >
      {showSkeleton && (
        <div className="mx-auto flex max-w-screen-xl flex-col gap-8" aria-hidden="true">
          <div className="h-10 w-2/5 animate-pulse rounded-md bg-surface-2" />
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 lg:grid-cols-6">
            {Array.from({ length: 6 }, (_, index) => (
              <div key={index} className="aspect-[2/3] animate-pulse rounded-lg bg-surface-2" />
            ))}
          </div>
        </div>
      )}
    </main>
  )
}
