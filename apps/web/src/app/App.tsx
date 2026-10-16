import type { i18n as I18n } from 'i18next'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'

import { createI18n } from '@/shared/i18n'
import { InputProvider } from '@/shared/input'

import { UiKit } from './UiKit'

/** Composition root of the UI. Adapters, providers and the router join it in later phases. */
export function App({ i18n: injected }: { i18n?: I18n }) {
  const [instance] = useState(() => injected ?? createI18n())
  return (
    <I18nextProvider i18n={instance}>
      <InputProvider>
        <UiKit />
      </InputProvider>
    </I18nextProvider>
  )
}
