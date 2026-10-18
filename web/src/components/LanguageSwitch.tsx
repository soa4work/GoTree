import { useTranslation } from 'react-i18next'
import { saveLanguage, selectableLanguages, type Language } from '../lib/language'

const names: Record<(typeof selectableLanguages)[number], string> = { ru: 'Русский', kk: 'Қазақша' }

/** Offers only ru and kk; the hidden English fallback is not selectable. */
export function LanguageSwitch() {
  const { t, i18n } = useTranslation()
  const current = selectableLanguages.find((l) => l === i18n.language) ?? 'ru'

  return (
    <>
      <label htmlFor="language-switch" className="sr-only">
        {t('language.label')}
      </label>
      <select
        id="language-switch"
        value={current}
        onChange={(e) => {
          const next = e.target.value as Language
          saveLanguage(next)
          void i18n.changeLanguage(next)
        }}
        className="min-h-11 rounded-lg border border-slate-300 bg-white px-2 text-sm dark:border-slate-700 dark:bg-slate-900"
      >
        {selectableLanguages.map((l) => (
          <option key={l} value={l} lang={l}>
            {names[l]}
          </option>
        ))}
      </select>
    </>
  )
}
