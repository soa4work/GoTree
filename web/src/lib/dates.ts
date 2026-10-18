import i18n from '../i18n'

const MONTHS = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']

// "en" alone would give the US order ("February 26, 1820"). With a day and a
// month, ru gives the genitive ("26 февраля 1820") through Intl.
const LOCALES: Record<string, string> = { en: 'en-GB', ru: 'ru-RU', kk: 'kk-KZ' }
const locale = () => LOCALES[i18n.language] ?? i18n.language

/**
 * Day, month and year in the interface language. Intl puts the words in the
 * right order and case ("26 February 1820", later "26 февраля 1820"). The
 * year is the text as written, so "1750/51" and years before 1000 survive.
 */
function longDate(year: string, month: number, day: number, dual = ''): string {
  const date = new Date(0)
  date.setUTCFullYear(Number(year), (month || 1) - 1, day || 1)
  const options: Intl.DateTimeFormatOptions = { year: 'numeric', timeZone: 'UTC' }
  if (month) options.month = 'long'
  if (day) options.day = 'numeric'
  return new Intl.DateTimeFormat(locale(), options)
    .formatToParts(date)
    .map((p) => (p.type === 'year' ? String(Number(year)) + dual : p.value))
    .join('')
}

/** One date in GEDCOM syntax ("26 FEB 1820", "FEB 1820", "1750/51", "12.3.1850", "@#DJULIAN@ 5 MAY 1700 B.C."), or null. */
function simple(text: string): string | null {
  let day = 0
  let month = 0
  let year = ''
  let dual = ''
  let calendar = ''
  let bc = false

  const iso = text.match(/^(\d{4})-(\d{2})(?:-(\d{2}))?$/)
  const dotted = text.match(/^(?:(\d{1,2})\.)?(\d{1,2})\.(\d{4})$/)
  if (iso) {
    ;[year, month, day] = [iso[1] ?? '', Number(iso[2]), Number(iso[3] ?? 0)]
  } else if (dotted) {
    ;[day, month, year] = [Number(dotted[1] ?? 0), Number(dotted[2]), dotted[3] ?? '']
  } else {
    const m = text.match(/^(@#DJULIAN@\s+|@#DGREGORIAN@\s+)?(?:(\d{1,2})\s+)?(?:([A-Z]{3})\s+)?(\d{1,4})(\/\d{2})?(\s+B\.C\.)?$/)
    if (!m) return null
    calendar = m[1] ?? ''
    day = Number(m[2] ?? 0)
    month = m[3] ? MONTHS.indexOf(m[3]) + 1 : 0
    if (m[3] && !month) return null
    year = m[4] ?? ''
    dual = m[5] ?? ''
    bc = Boolean(m[6])
  }
  if (month < 0 || month > 12 || (day && !month)) return null

  let out = longDate(year, month, day, dual)
  if (bc) out = i18n.t('date.fmt.bc', { date: out })
  if (calendar.includes('JULIAN')) out = i18n.t('date.fmt.julian', { date: out })
  return out
}

const QUALIFIERS = {
  ABT: 'about',
  CAL: 'calculated',
  EST: 'estimated',
  BEF: 'before',
  AFT: 'after',
  FROM: 'from',
  TO: 'to',
} as const

/**
 * A GEDCOM date as plain English: "26 FEB 1820" → "26 February 1820",
 * "BET 1850 AND 1860" → "between 1850 and 1860". Dates typed in the forms
 * the server also reads ("abt 1880", "about 1880", "12.3.1850") are shown the
 * same way; text that is not a date is returned unchanged.
 */
export function formatDate(gedcom: string): string {
  const text = gedcom.trim()
  if (!text) return ''
  const t = i18n.t.bind(i18n)
  const simpleOf = (part: string | undefined) => simple(part ?? '')

  if (/^\(.*\)$/.test(text)) return text

  // Only an INT date has a phrase; it is free text and keeps its case.
  const withPhrase = text.match(/^([^(]+?)\s*\((.*)\)$/)
  const phrase = withPhrase?.[2]
  const src = (withPhrase?.[1] ?? text)
    .toUpperCase()
    .replace(/^ABOUT\b/, 'ABT')
    .replace(/^BEFORE\b/, 'BEF')
    .replace(/^AFTER\b/, 'AFT')
    .replace(/^BETWEEN\b/, 'BET')

  if (phrase !== undefined && !src.startsWith('INT ')) return text

  const one = simple(src)
  if (one !== null) return one

  const between = src.match(/^BET\s+(.+?)\s+AND\s+(.+)$/)
  if (between) {
    const [a, b] = [simpleOf(between[1]), simpleOf(between[2])]
    return a && b ? t('date.fmt.between', { start: a, end: b }) : text
  }
  const fromTo = src.match(/^FROM\s+(.+?)\s+TO\s+(.+)$/)
  if (fromTo) {
    const [a, b] = [simpleOf(fromTo[1]), simpleOf(fromTo[2])]
    return a && b ? t('date.fmt.fromTo', { start: a, end: b }) : text
  }
  const interpreted = src.match(/^INT\s+(.+)$/)
  if (interpreted) {
    const d = simpleOf(interpreted[1])
    if (!d) return text
    return phrase ? t('date.fmt.interpreted', { date: d, phrase }) : d
  }
  const qualified = src.match(/^(ABT|CAL|EST|BEF|AFT|FROM|TO)\s+(.+)$/)
  if (qualified) {
    const d = simpleOf(qualified[2])
    if (!d) return text
    return t(`date.fmt.${QUALIFIERS[qualified[1] as keyof typeof QUALIFIERS]}`, { date: d })
  }
  return text
}

/** A timestamp or "YYYY-MM-DD" day (created, expires, due) in the interface language, in the same style as formatDate. */
export function formatDay(iso: string): string {
  const date = new Date(/^\d{4}-\d{2}-\d{2}$/.test(iso) ? `${iso}T00:00:00` : iso)
  if (Number.isNaN(date.getTime())) return iso
  return longDate(String(date.getFullYear()), date.getMonth() + 1, date.getDate())
}
