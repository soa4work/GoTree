import i18n from '../i18n'

const MONTHS = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']

const monthName = (month: number): string =>
  new Date(Date.UTC(2000, month - 1, 1)).toLocaleDateString(i18n.language, { month: 'long', timeZone: 'UTC' })

/** One calendar date in GEDCOM syntax ("26 FEB 1820", "FEB 1820", "1750/51", "@#DJULIAN@ 5 MAY 1700 B.C."), or null. */
function simple(text: string): string | null {
  const m = text.match(/^(@#DJULIAN@\s+|@#DGREGORIAN@\s+)?(?:(\d{1,2})\s+)?(?:([A-Z]{3})\s+)?(\d{1,4})(\/\d{2})?(\s+B\.C\.)?$/)
  if (!m) return null
  const [, calendar, day, mon, year, dual, bc] = m
  const month = mon ? MONTHS.indexOf(mon) + 1 : 0
  if (mon && !month) return null
  if (day && !month) return null
  const parts = [day ? String(Number(day)) : '', month ? monthName(month) : '', year + (dual ?? '')].filter(Boolean)
  let out = parts.join(' ')
  if (bc) out = i18n.t('date.fmt.bc', { date: out })
  if (calendar?.includes('JULIAN')) out = i18n.t('date.fmt.julian', { date: out })
  return out
}

/**
 * A GEDCOM date as plain English: "26 FEB 1820" → "26 February 1820",
 * "BET 1850 AND 1860" → "between 1850 and 1860". Text that is not GEDCOM
 * is returned unchanged.
 */
export function formatDate(gedcom: string): string {
  const text = gedcom.trim()
  if (!text) return ''
  const t = i18n.t.bind(i18n)
  const simpleOf = (part: string | undefined) => simple(part ?? '')

  const one = simple(text)
  if (one !== null) return one

  const between = text.match(/^BET\s+(.+?)\s+AND\s+(.+)$/)
  if (between) {
    const [a, b] = [simpleOf(between[1]), simpleOf(between[2])]
    return a && b ? t('date.fmt.between', { start: a, end: b }) : text
  }
  const fromTo = text.match(/^FROM\s+(.+?)\s+TO\s+(.+)$/)
  if (fromTo) {
    const [a, b] = [simpleOf(fromTo[1]), simpleOf(fromTo[2])]
    return a && b ? t('date.fmt.fromTo', { start: a, end: b }) : text
  }
  const phrase = text.match(/^\((.*)\)$/)
  if (phrase) return text
  const interpreted = text.match(/^INT\s+(.+?)(?:\s+\((.*)\))?$/)
  if (interpreted) {
    const d = simpleOf(interpreted[1])
    if (!d) return text
    return interpreted[2] ? t('date.fmt.interpreted', { date: d, phrase: interpreted[2] }) : d
  }
  const qualified = text.match(/^(ABT|CAL|EST|BEF|AFT|FROM|TO)\s+(.+)$/)
  if (qualified) {
    const d = simpleOf(qualified[2])
    if (!d) return text
    const keys = { ABT: 'about', CAL: 'calculated', EST: 'estimated', BEF: 'before', AFT: 'after', FROM: 'from', TO: 'to' } as const
    return t(`date.fmt.${keys[qualified[1] as keyof typeof keys]}`, { date: d })
  }
  return text
}

/** A timestamp or "YYYY-MM-DD" day (created, expires, due) in the interface language. */
export function formatDay(iso: string): string {
  const date = new Date(/^\d{4}-\d{2}-\d{2}$/.test(iso) ? `${iso}T00:00:00` : iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleDateString(i18n.language, { dateStyle: 'long' })
}
