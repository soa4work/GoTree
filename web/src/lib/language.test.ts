import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { formatDay } from './dates'
import { detectLanguage, languageStorageKey } from './language'

afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
})

describe('detectLanguage', () => {
  it('prefers the saved choice, including the hidden en', () => {
    localStorage.setItem(languageStorageKey, 'kk')
    expect(detectLanguage()).toBe('kk')
    localStorage.setItem(languageStorageKey, 'en')
    expect(detectLanguage()).toBe('en')
  })

  it('follows ru and kk browsers, and falls back to ru for the rest', () => {
    vi.stubGlobal('navigator', { language: 'kk-KZ', languages: ['kk-KZ'] })
    expect(detectLanguage()).toBe('kk')
    vi.stubGlobal('navigator', { language: 'en-US', languages: ['en-US'] })
    expect(detectLanguage()).toBe('ru')
  })

  it('ignores an unknown saved value', () => {
    localStorage.setItem(languageStorageKey, 'de')
    vi.stubGlobal('navigator', { language: 'ru-RU', languages: ['ru-RU'] })
    expect(detectLanguage()).toBe('ru')
  })
})

describe('language change', () => {
  it('keeps <html lang> in sync and formats days in the genitive for ru', async () => {
    await i18n.changeLanguage('ru')
    expect(document.documentElement.lang).toBe('ru')
    expect(formatDay('2026-03-05')).toMatch(/^5 марта 2026/)
    await i18n.changeLanguage('kk')
    expect(document.documentElement.lang).toBe('kk')
    await i18n.changeLanguage('en')
    expect(document.documentElement.lang).toBe('en')
  })
})
