import { useTranslation } from 'react-i18next'

import { useUploadUi } from './UploadUiContext'

export function TryExamplesButton({ className }: { className: string }) {
  const { t } = useTranslation()
  const { openExamples } = useUploadUi()
  return (
    <button
      type="button"
      className={`min-h-control-sm items-center justify-center rounded-pill border border-control px-3 text-center text-caption font-bold md:text-body-sm ${className}`}
      onClick={openExamples}
    >
      {t('uploads.tryExamples')}
    </button>
  )
}
