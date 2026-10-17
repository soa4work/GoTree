import { describe, expect, it } from 'vitest'
import { formatDate, formatDay } from './dates'

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
    ['31 FEB', '31 FEB'],
    ['ABT sometime', 'ABT sometime'],
  ])('formatDate(%j) = %j', (gedcom, want) => {
    expect(formatDate(gedcom)).toBe(want)
  })
})

describe('formatDay', () => {
  it('formats timestamps and plain days the same way', () => {
    expect(formatDay('2026-03-05')).toBe('March 5, 2026')
    expect(formatDay('2026-03-05T12:00:00Z')).toMatch(/^March [45], 2026$/)
  })

  it('returns text it cannot read unchanged', () => {
    expect(formatDay('soon')).toBe('soon')
  })
})
