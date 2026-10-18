import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { detectLanguage } from './lib/language'
import en from './locales/en.json'
import kk from './locales/kk.json'
import ru from './locales/ru.json'

export const defaultNS = 'translation'
// Key types come from en (see i18next.d.ts); ru and kk are tested to have the same keys.
export const resources = { en: { translation: en }, ru: { translation: ru }, kk: { translation: kk } } as const

/** Keeps the page's language and its meta description in step with the interface language. */
function syncDocument(lang: string) {
  if (typeof document === 'undefined') return
  document.documentElement.lang = lang
  document.querySelector('meta[name="description"]')?.setAttribute('content', i18n.t('app.description'))
}

void i18n.use(initReactI18next).init({
  resources,
  lng: detectLanguage(),
  fallbackLng: 'en',
  defaultNS,
  interpolation: { escapeValue: false },
})

syncDocument(i18n.language)
i18n.on('languageChanged', syncDocument)

export default i18n
