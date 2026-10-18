import { afterEach, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { custodyPeriod } from './custodyText'

afterEach(() => i18n.changeLanguage('en'))

const period = (fromDate: string, toDate: string) => custodyPeriod(i18n.t.bind(i18n), { fromDate, toDate })

describe('custodyPeriod', () => {
  it('reads in English', async () => {
    await i18n.changeLanguage('en')
    expect(period('1965', '')).toBe('since 1965')
    expect(period('', 'FEB 1965')).toBe('until February 1965')
    expect(period('', '')).toBe('')
  })

  it('declines a lone Russian month after с and до', async () => {
    await i18n.changeLanguage('ru')
    expect(period('FEB 1920', '')).toBe('с февраля 1920 г.')
    expect(period('', 'MAR 1930')).toBe('до марта 1930 г.')
    expect(period('FEB 1920', 'MAR 1930')).toBe('февраль 1920 г. – март 1930 г.')
  })

  it('writes «ж.» once in Kazakh', async () => {
    await i18n.changeLanguage('kk')
    expect(period('1965', '')).toBe('1965 ж. бастап')
    expect(period('', '26 FEB 1965')).toBe('1965 ж. 26 ақпан дейін')
    expect(period('1920', '1965')).toBe('1920 ж. – 1965 ж.')
  })
})
