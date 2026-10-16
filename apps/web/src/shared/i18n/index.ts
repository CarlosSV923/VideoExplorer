import i18next, { type i18n } from 'i18next'
import { initReactI18next } from 'react-i18next'

import en from './en.json'
import es from './es.json'

export const languages = ['es', 'en'] as const
export type Language = (typeof languages)[number]

export const defaultLanguage: Language = 'es'

declare module 'i18next' {
  interface CustomTypeOptions {
    resources: { translation: typeof es }
  }
}

const storageKey = 've.lang'

export function isLanguage(value: unknown): value is Language {
  return languages.includes(value as Language)
}

/** The language remembered in this browser, if any (RF-61). */
export function storedLanguage(): Language {
  try {
    const value = localStorage.getItem(storageKey)
    return isLanguage(value) ? value : defaultLanguage
  } catch {
    return defaultLanguage // storage blocked (private mode, sandbox)
  }
}

function rememberLanguage(language: Language) {
  try {
    localStorage.setItem(storageKey, language)
  } catch {
    // The choice still applies to this visit.
  }
}

/** Creates an initialized i18n instance. Resources are bundled: no network. */
export function createI18n(language: Language = storedLanguage()): i18n {
  const instance = i18next.createInstance()
  void instance.use(initReactI18next).init({
    resources: { es: { translation: es }, en: { translation: en } },
    lng: language,
    fallbackLng: defaultLanguage,
    supportedLngs: languages,
    interpolation: { escapeValue: false }, // React escapes
    initAsync: false,
  })
  document.documentElement.lang = language
  return instance
}

/** Switches the language, remembers it and updates <html lang>. */
export async function changeLanguage(instance: i18n, language: Language) {
  await instance.changeLanguage(language)
  rememberLanguage(language)
  document.documentElement.lang = language
}

export { formatDate, formatDateTime, formatDuration, formatRelative, formatSize } from './format'
