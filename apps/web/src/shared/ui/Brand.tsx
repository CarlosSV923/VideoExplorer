import { useTranslation } from 'react-i18next'

export function Brand() {
  const { t } = useTranslation()
  const name = t('app.name')

  return (
    <a
      href="/"
      aria-label={name}
      className="inline-flex min-h-control-sm items-center whitespace-nowrap text-[20px] leading-none font-display normal-case no-underline sm:text-[22px]"
    >
      <span className="text-ink-1">{name.slice(0, 5)}</span>
      <span className="text-accent">{name.slice(5)}</span>
    </a>
  )
}
