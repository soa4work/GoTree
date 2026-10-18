import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { formatDate, formatDay, formatDayMonth } from './dates'

afterEach(async () => {
  vi.unstubAllGlobals()
  await i18n.changeLanguage('en')
})

const RU_GEN = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря']
const KK = ['қаңтар', 'ақпан', 'наурыз', 'сәуір', 'мамыр', 'маусым', 'шілде', 'тамыз', 'қыркүйек', 'қазан', 'қараша', 'желтоқсан']
const MON = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']

const ruCases: [string, string][] = [
  ['1850', '1850'],
  ['FEB 1820', 'февраль 1820 г.'],
  ['26 FEB 1820', '26 февраля 1820 г.'],
  ['ABT FEB 1820', 'около февраля 1820 г.'],
  ['CAL FEB 1820', 'вычислено: февраль 1820 г.'],
  ['EST FEB 1820', 'по оценке: февраль 1820 г.'],
  ['BEF FEB 1820', 'до февраля 1820 г.'],
  ['BEF 26 FEB 1820', 'до 26 февраля 1820 г.'],
  ['BEF 1900', 'до 1900'],
  ['AFT MAY 1900', 'после мая 1900 г.'],
  ['FROM AUG 1914', 'с августа 1914 г.'],
  ['TO NOV 1918', 'по ноябрь 1918 г.'],
  ['FROM AUG 1914 TO NOV 1918', 'с августа 1914 г. по ноябрь 1918 г.'],
  ['BET JAN 1850 AND FEB 1851', 'между январём 1850 г. и февралём 1851 г.'],
  ['BET 26 FEB 1850 AND 1860', 'между 26 февраля 1850 г. и 1860'],
  ['BET 1850 AND 1860', 'между 1850 и 1860'],
  ['INT FEB 1820 (from a letter)', 'февраль 1820 г. (from a letter)'],
  ['10 FEB 1750/51', '10 февраля 1750/51 г.'],
  ['500 B.C.', '500 до н. э.'],
  ['BEF MAR 500 B.C.', 'до марта 500 г. до н. э.'],
  ['@#DJULIAN@ 5 MAY 1700', '5 мая 1700 г. (юлианский календарь)'],
  ['sometime in spring', 'sometime in spring'],
  ...MON.map((m, i): [string, string] => [`BEF ${m} 1900`, `до ${RU_GEN[i]} 1900 г.`]),
]

const kkCases: [string, string][] = [
  ['1850', '1850 ж.'],
  ['FEB 1820', '1820 ж. ақпан'],
  ['26 FEB 1820', '1820 ж. 26 ақпан'],
  ['BEF 26 FEB 1820', '1820 ж. 26 ақпан дейін'],
  ['BEF 1820', '1820 ж. дейін'],
  ['AFT FEB 1820', '1820 ж. ақпан кейін'],
  ['FROM 1914', '1914 ж. бастап'],
  ['TO 1918', '– 1918 ж.'],
  ['FROM 1914 TO 1918', '1914 ж. – 1918 ж.'],
  ['BET 1850 AND 1860', '1850 ж. – 1860 ж. аралығында'],
  ['ABT 1850', 'шамамен 1850 ж.'],
  ['10 FEB 1750/51', '1750/51 ж. 10 ақпан'],
  ['500 B.C.', '500 ж. б.з.д.'],
  ['@#DJULIAN@ 5 MAY 1700', '1700 ж. 5 мамыр (Юлиан күнтізбесі)'],
  ...MON.map((m, i): [string, string] => [`${m} 1900`, `1900 ж. ${KK[i]}`]),
]

/** Years in the input, to check that every one gets exactly one "ж.". */
const yearsIn = (gedcom: string) => (gedcom.match(/\b\d{3,4}\b/g) ?? []).length

describe('in Russian', () => {
  beforeEach(() => i18n.changeLanguage('ru'))

  it.each(ruCases)('formatDate(%j) = %j', (gedcom, want) => {
    expect(formatDate(gedcom)).toBe(want)
  })

  it('declines a lone month by its context, but only for a simple date', () => {
    expect(formatDate('FEB 1920', 'since')).toBe('февраля 1920 г.')
    expect(formatDate('MAR 1930', 'until')).toBe('марта 1930 г.')
    expect(formatDate('ABT 1920', 'since')).toBe('около 1920')
    expect(formatDate('FEB 1920')).toBe('февраль 1920 г.')
  })

  it('formats days', () => {
    expect(formatDay('2026-03-05')).toBe('5 марта 2026 г.')
    expect(formatDayMonth(10, 18)).toBe('18 октября')
  })
})

describe('in Kazakh', () => {
  beforeEach(() => i18n.changeLanguage('kk'))

  it.each(kkCases)('formatDate(%j) = %j', (gedcom, want) => {
    const out = formatDate(gedcom)
    expect(out).toBe(want)
    expect((out.match(/ж\./g) ?? []).length).toBe(yearsIn(gedcom))
  })

  it('formats days', () => {
    expect(formatDay('2026-03-05')).toBe('2026 ж. 5 наурыз')
    expect(formatDayMonth(10, 18)).toBe('18 қазан')
  })
})

describe('without ru and kk data in Intl', () => {
  beforeEach(() => {
    const Real = Intl.DateTimeFormat
    const Fake = function (_locales?: string | string[], options?: Intl.DateTimeFormatOptions) {
      return new Real('en-US', options)
    } as unknown as typeof Intl.DateTimeFormat
    Fake.supportedLocalesOf = (l: string | string[]) => [l].flat().filter((x) => x.startsWith('en'))
    // Intl's members are not enumerable, so a spread would drop them.
    vi.stubGlobal('Intl', Object.create(Intl, { DateTimeFormat: { value: Fake } }))
  })

  it('really has no kk data', () => {
    expect(Intl.DateTimeFormat.supportedLocalesOf(['kk'])).toEqual([])
  })

  it.each(ruCases)('ru formatDate(%j) = %j', async (gedcom, want) => {
    await i18n.changeLanguage('ru')
    expect(formatDate(gedcom)).toBe(want)
  })

  it.each(kkCases)('kk formatDate(%j) = %j', async (gedcom, want) => {
    await i18n.changeLanguage('kk')
    expect(formatDate(gedcom)).toBe(want)
  })

  it('kk days are not in English', async () => {
    await i18n.changeLanguage('kk')
    expect(formatDay('2026-03-05')).toBe('2026 ж. 5 наурыз')
    expect(formatDayMonth(3, 5)).toBe('5 наурыз')
  })
})

describe('formatDayMonth', () => {
  it('writes the day before the month in English', () => {
    expect(formatDayMonth(10, 18)).toBe('18 October')
    expect(formatDayMonth(2, 29)).toBe('29 February')
  })
})

describe('formatDate', () => {
  it.each([
    ['', ''],
    ['1850', '1850'],
    ['FEB 1820', 'February 1820'],
    ['26 FEB 1820', '26 February 1820'],
    ['5 MAY 1700', '5 May 1700'],
    ['ABT 1850', 'about 1850'],
    ['ABT 12 MAR 1850', 'about 12 March 1850'],
    ['CAL 1850', 'calculated 1850'],
    ['EST 1850', 'estimated 1850'],
    ['BEF 1900', 'before 1900'],
    ['AFT 1900', 'after 1900'],
    ['BET 1850 AND 1860', 'between 1850 and 1860'],
    ['BET JAN 1850 AND FEB 1851', 'between January 1850 and February 1851'],
    ['FROM 1914', 'from 1914'],
    ['TO 1918', 'until 1918'],
    ['FROM 1914 TO 1918', 'from 1914 to 1918'],
    ['INT 1850 (family bible)', '1850 (family bible)'],
    ['INT 1850', '1850'],
    ['10 FEB 1750/51', '10 February 1750/51'],
    ['500 B.C.', '500 BC'],
    ['@#DJULIAN@ 5 MAY 1700', '5 May 1700 (Julian calendar)'],
    ['(during the war)', '(during the war)'],
    ['sometime in spring', 'sometime in spring'],
    ['abt 1880', 'about 1880'],
    ['about 1880', 'about 1880'],
    ['before 1900', 'before 1900'],
    ['12.3.1850', '12 March 1850'],
    ['1850-03-12', '12 March 1850'],
    ['31 FEB', '31 FEB'],
    ['ABT sometime', 'ABT sometime'],
  ])('formatDate(%j) = %j', (gedcom, want) => {
    expect(formatDate(gedcom)).toBe(want)
  })
})

describe('formatDay', () => {
  it('formats timestamps and plain days the same way', () => {
    expect(formatDay('2026-03-05')).toBe('5 March 2026')
    expect(formatDay('2026-03-05T12:00:00Z')).toMatch(/^[45] March 2026$/)
  })

  it('returns text it cannot read unchanged', () => {
    expect(formatDay('soon')).toBe('soon')
  })
})
