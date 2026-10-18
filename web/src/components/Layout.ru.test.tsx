import { screen, within } from '@testing-library/react'
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { mockApi, renderApp } from '../test/render'

describe('Russian interface', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('ru')
  })
  afterAll(async () => {
    await i18n.changeLanguage('en')
  })

  it('shows the navigation in Russian', async () => {
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: true, media: query, addEventListener: () => {}, removeEventListener: () => {} }))
    mockApi({
      'GET /api/stats': [200, { persons: 1, families: 0, sources: 0 }],
      'GET /api/research/tasks': [200, []],
      'GET /api/onthisday': [200, { events: [] }],
    })
    renderApp('/')
    const nav = await screen.findByRole('navigation', { name: 'Основная навигация' })
    const links = within(nav)
      .getAllByRole('link')
      .map((a) => a.textContent)
    expect(links).toContain('Главная')
    expect(links).toContain('Настенная схема')
    expect(links).toContain('Карта переселений')
    expect(links).toContain('Участники')
    expect(await screen.findByRole('heading', { name: 'Добро пожаловать в GoTree' })).toBeInTheDocument()
  })

  it('picks the Russian plural form', () => {
    const t = i18n.getFixedT('ru')
    expect(t('people.count', { count: 1 })).toBe('1 человек')
    expect(t('people.count', { count: 2 })).toBe('2 человека')
    expect(t('people.count', { count: 5 })).toBe('5 человек')
    expect(t('people.count', { count: 21 })).toBe('21 человек')
    expect(t('people.count', { count: 22 })).toBe('22 человека')
  })
})
