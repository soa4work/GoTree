import { afterEach, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { compareText, formatDecimal } from './collate'

afterEach(async () => {
  await i18n.changeLanguage('en')
})

describe('compareText', () => {
  it('orders Kazakh with Ы before І', async () => {
    await i18n.changeLanguage('kk')
    expect(['Іле', 'Ыстық'].sort(compareText)).toEqual(['Ыстық', 'Іле'])
  })

  it('orders Russian with Ё after Е', async () => {
    await i18n.changeLanguage('ru')
    expect(['Жуков', 'Ёлкин', 'Елисеев'].sort(compareText)).toEqual(['Елисеев', 'Ёлкин', 'Жуков'])
  })

  it('ignores case and sorts numbers by value', async () => {
    await i18n.changeLanguage('ru')
    expect(['b10', 'B2', 'a'].sort(compareText)).toEqual(['a', 'B2', 'b10'])
  })
})

describe('formatDecimal', () => {
  it('uses the decimal comma of Russian and Kazakh', async () => {
    await i18n.changeLanguage('ru')
    expect(formatDecimal(1.5, 1)).toBe('1,5')
    await i18n.changeLanguage('kk')
    expect(formatDecimal(1.5, 1)).toBe('1,5')
  })

  it('keeps the point in English and pads the decimals', async () => {
    await i18n.changeLanguage('en')
    expect(formatDecimal(1.5, 1)).toBe('1.5')
    expect(formatDecimal(2, 1)).toBe('2.0')
  })
})
