export type Language = 'ru' | 'kk' | 'en'

/** The languages the switcher offers. English stays a hidden fallback and is never listed. */
export const selectableLanguages = ['ru', 'kk'] as const
export type SelectableLanguage = (typeof selectableLanguages)[number]

/** The localStorage key of the saved choice; unit tests and Playwright set it to 'en'. */
export const languageStorageKey = 'gotree-lang'

function isLanguage(v: unknown): v is Language {
  return v === 'ru' || v === 'kk' || v === 'en'
}

export function storedLanguage(): Language | null {
  try {
    const v = localStorage.getItem(languageStorageKey)
    return isLanguage(v) ? v : null
  } catch {
    return null
  }
}

/**
 * The saved choice, then the browser language (ru or kk), else Russian.
 * A saved 'en' is honoured although the switcher never offers it: unit
 * tests and Playwright set it, so they keep running on the English source
 * strings. A browser that merely prefers English still gets Russian.
 */
export function detectLanguage(): Language {
  const saved = storedLanguage()
  if (saved) return saved
  const langs = typeof navigator === 'undefined' ? [] : [...(navigator.languages ?? []), navigator.language]
  for (const l of langs) {
    const prefix = l?.toLowerCase().slice(0, 2)
    if (prefix === 'ru' || prefix === 'kk') return prefix
  }
  return 'ru'
}

export function saveLanguage(lang: Language) {
  try {
    localStorage.setItem(languageStorageKey, lang)
  } catch {
    // Without storage the choice lasts until the page is reloaded.
  }
}
