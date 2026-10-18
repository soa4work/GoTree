import { afterEach, describe, expect, it } from 'vitest'
import i18n from './i18n'
import kk from './locales/kk.json'

afterEach(() => i18n.changeLanguage('en'))

describe('document language', () => {
  it('follows the interface language, including the meta description', async () => {
    const meta = document.createElement('meta')
    meta.name = 'description'
    document.head.append(meta)
    await i18n.changeLanguage('kk')
    expect(document.documentElement.lang).toBe('kk')
    expect(meta.content).toBe(kk.app.description)
    meta.remove()
  })
})
