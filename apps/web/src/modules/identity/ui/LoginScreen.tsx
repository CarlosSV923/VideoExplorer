import { useState, type SyntheticEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { isAppError } from '@/shared/kernel/errors'
import { Banner, Brand, Button, PasswordField } from '@/shared/ui'

import { useIdentityPorts } from '../application/ports'

export function LoginScreen({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation()
  const identity = useIdentityPorts()
  const [password, setPassword] = useState('')
  const [error, setError] = useState<unknown>()
  const [loading, setLoading] = useState(false)

  const submit = async (event: SyntheticEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError(undefined)
    setLoading(true)
    try {
      await identity.login(password)
      onSuccess()
    } catch (caught) {
      setError(caught)
    } finally {
      setLoading(false)
    }
  }

  const errorText = isAppError(error, 'rateLimited')
    ? t('errors.rateLimited')
    : isAppError(error, 'unauthorized')
      ? t('errors.loginUnauthorized')
      : error
        ? t('errors.loginFailed')
        : undefined

  return (
    <main className="flex min-h-app items-center justify-center p-6">
      <section className="flex w-full max-w-md flex-col gap-8 rounded-xl border border-line bg-surface-1 p-8">
        <header className="flex flex-col gap-2">
          <Brand />
          <h1 className="m-0 text-heading font-bold">{t('login.title')}</h1>
          <p className="m-0 text-body text-ink-2">{t('login.body')}</p>
        </header>
        <form
          className="flex flex-col gap-5"
          onSubmit={(event) => {
            void submit(event)
          }}
        >
          {errorText && <Banner tone="danger">{errorText}</Banner>}
          <PasswordField
            label={t('login.password')}
            value={password}
            onChange={(event) => {
              setPassword(event.currentTarget.value)
            }}
            autoComplete="current-password"
            autoFocus
          />
          <Button type="submit" variant="primary" loading={loading} disabled={!password}>
            {t('login.submit')}
          </Button>
        </form>
      </section>
    </main>
  )
}
