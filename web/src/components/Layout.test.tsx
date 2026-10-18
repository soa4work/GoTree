import { fireEvent, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

const home: Parameters<typeof mockApi>[0] = {
  'GET /api/stats': [200, { persons: 1, families: 0, sources: 0 }],
  'GET /api/research/tasks': [200, []],
  'GET /api/onthisday': [200, { events: [] }],
}

// jsdom has no matchMedia; the layout shows the sidebar from 48rem up and the phone bar below.
async function render(path = '/', wide = true) {
  vi.stubGlobal('matchMedia', (query: string) => ({ matches: wide, media: query, addEventListener: () => {}, removeEventListener: () => {} }))
  mockApi(home)
  const view = renderApp(path)
  const nav = await screen.findByRole('navigation', { name: 'Main navigation' })
  return { ...view, nav }
}

const names = (el: HTMLElement) => within(el).getAllByRole('link').map((a) => a.textContent)

describe('navigation', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('groups the sidebar under headings, with Home on top and the wall chart in', async () => {
    const { nav: sidebar } = await render()
    expect(names(sidebar)[0]).toBe('Home')
    const expected: Record<string, string[]> = {
      Tree: ['People', 'Tree', 'Wall chart', 'Relationships'],
      Records: ['Sources', 'Photos & media', 'Transcribe', 'Heirlooms'],
      Explore: ['Migration map', 'Statistics'],
      Work: ['Research', 'Data quality'],
      Settings: ['Sharing', 'Members', 'Import & export', 'Account'],
    }
    for (const [group, links] of Object.entries(expected)) {
      expect(names(within(sidebar).getByRole('list', { name: group }))).toEqual(links)
    }
    expect(within(sidebar).getByRole('link', { name: 'Home' })).toHaveAttribute('aria-current', 'page')
  })

  it('marks the current page', async () => {
    const { nav: sidebar } = await render('/chart')
    expect(within(sidebar).getByRole('link', { name: 'Wall chart' })).toHaveAttribute('aria-current', 'page')
    expect(within(sidebar).getByRole('link', { name: 'Home' })).not.toHaveAttribute('aria-current')
  })

  it('keeps four links and a More button on the phone bar', async () => {
    const { nav: phone } = await render('/', false)
    expect(names(phone)).toEqual(['Home', 'People', 'Tree', 'Sources'])
    expect(within(phone).getByRole('button', { name: 'More' })).toHaveAttribute('aria-haspopup', 'dialog')
    expect(within(phone).queryByRole('link', { name: 'Import & export' })).not.toBeInTheDocument()
  })

  it('opens More with every other section, closes on Escape', async () => {
    const user = userEvent.setup()
    const { nav: phone, container } = await render('/', false)
    const more = within(phone).getByRole('button', { name: 'More' })
    await user.click(more)
    const sheet = screen.getByRole('dialog', { name: 'More' })
    expect(within(sheet).getByRole('navigation', { name: 'More sections' })).toBeInTheDocument()
    expect(within(sheet).getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Tree', 'Records', 'Explore', 'Work', 'Settings'])
    expect(within(sheet).getByRole('link', { name: 'Sharing' })).toHaveAttribute('href', '/sharing')
    expect(within(sheet).getByRole('link', { name: 'Wall chart' })).toHaveAttribute('href', '/chart')
    expect(await axeViolations(container)).toEqual([])

    // Escape is the dialog's cancel event; the browser restores focus to the button.
    fireEvent(sheet, new Event('cancel', { cancelable: true }))
    expect(screen.queryByRole('dialog', { name: 'More' })).not.toBeInTheDocument()
  })

  it('closes More after choosing a section', async () => {
    const user = userEvent.setup()
    const { nav: phone } = await render('/', false)
    await user.click(within(phone).getByRole('button', { name: 'More' }))
    await user.click(within(screen.getByRole('dialog', { name: 'More' })).getByRole('link', { name: 'Sharing' }))
    expect(screen.queryByRole('dialog', { name: 'More' })).not.toBeInTheDocument()
  })
})

describe('logging out', () => {
  afterEach(() => vi.unstubAllGlobals())

  async function renderWith(logoutStatus: number) {
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: true, media: query, addEventListener: () => {}, removeEventListener: () => {} }))
    const fetchMock = mockApi({ ...home, 'POST /api/auth/logout': [logoutStatus, { error: 'nope' }] })
    renderApp('/')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Log out' }))
    return fetchMock
  }

  it('says so when logging out fails', async () => {
    await renderWith(500)
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not log out. Try again.')
  })

  it('treats a 401 as logged out and re-checks the session', async () => {
    const fetchMock = await renderWith(401)
    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([u]) => String(u).endsWith('/api/auth/state')).length).toBeGreaterThan(1))
    expect(screen.queryByText('Could not log out. Try again.')).not.toBeInTheDocument()
  })
})
