import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { kinshipName } from '../relationship/name'
import { mockApi, renderApp } from '../test/render'

// The suite runs in English; this file renders the layout in Kazakh and restores English after.
describe('Kazakh interface', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('kk')
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: true, media: query, addEventListener: () => {}, removeEventListener: () => {} }))
    mockApi({
      'GET /api/stats': [200, { persons: 1, families: 0, sources: 0 }],
      'GET /api/research/tasks': [200, []],
      'GET /api/onthisday': [200, { events: [] }],
    })
  })
  afterEach(async () => {
    vi.unstubAllGlobals()
    await i18n.changeLanguage('en')
  })

  it('shows the navigation in Kazakh', async () => {
    renderApp('/')
    const nav = await screen.findByRole('navigation', { name: 'Негізгі навигация' })
    const names = within(nav).getAllByRole('link').map((a) => a.textContent)
    expect(names).toContain('Басты бет')
    expect(names).toContain('Шежіре ағашы')
    expect(names).toContain('Көші-қон картасы')
    expect(names).toContain('Мүшелер')
    expect(names).toContain('Тіркелгілер')
    expect(names).toContain('Деректер сапасы')
    expect(document.documentElement.lang).toBe('kk')
  })

  it('builds relationship names and sentences without suffixes on names', () => {
    const t = i18n.t.bind(i18n)
    expect(kinshipName(t, { up: 2, down: 0, half: false, adoptive: false }, 'F')).toBe('әже')
    expect(kinshipName(t, { up: 3, down: 0, half: false, adoptive: false }, 'M')).toBe('арғы ата')
    expect(t('relationship.sentence', { a: 'Айгүл', b: 'Нұрлан', relation: 'ұл' })).toBe('Айгүл үшін Нұрлан — ұл.')
    expect(t('people.count', { count: 5 })).toBe('5 адам')
  })
})
