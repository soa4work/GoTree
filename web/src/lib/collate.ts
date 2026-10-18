import i18n from '../i18n'

const collators = new Map<string, Intl.Collator>()
const formats = new Map<string, Intl.NumberFormat>()

function currentLanguage(): string {
  return (i18n.resolvedLanguage ?? i18n.language ?? 'ru').split('-')[0] || 'ru'
}

/** Compares two texts in the alphabet order of the interface language (Kazakh puts Ы before І). */
export function compareText(a: string, b: string): number {
  const lang = currentLanguage()
  let c = collators.get(lang)
  if (!c) {
    c = new Intl.Collator(lang, { sensitivity: 'base', numeric: true })
    collators.set(lang, c)
  }
  return c.compare(a, b)
}

/** A number with a fixed count of decimals and the decimal separator of the interface language ("1,5" in Russian and Kazakh). */
export function formatDecimal(n: number, digits: number): string {
  const lang = currentLanguage()
  const key = `${lang}/${digits}`
  let f = formats.get(key)
  if (!f) {
    f = new Intl.NumberFormat(lang, { minimumFractionDigits: digits, maximumFractionDigits: digits })
    formats.set(key, f)
  }
  return f.format(n)
}
