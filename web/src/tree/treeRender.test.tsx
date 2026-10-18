import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PersonRef, TreeFamily, TreeGraph } from '../api/types'
import { lifespan } from '../lib/people'
import { mockApi, renderApp } from '../test/render'

vi.mock('../lib/people', async (orig) => {
  const m = await orig<typeof import('../lib/people')>()
  return { ...m, lifespan: vi.fn(m.lifespan) }
})

const person = (id: number, givenNames: string): PersonRef => ({
  id,
  givenNames,
  surname: 'Weber',
  sex: 'U',
  birthDate: '1900',
  deathDate: '',
  living: false,
  portrait: null,
})

const link = (id: number, p1: number, p2: number, child: number): TreeFamily => ({
  id,
  partner1Id: p1,
  partner2Id: p2,
  unionType: 'married',
  children: [{ personId: child, relationPartner1: 'birth', relationPartner2: 'birth' }],
})

const names = ['Paul', 'Hans', 'Maria', 'Karl', 'Anna', 'Otto', 'Ida', 'Eva', 'Mia']
const graph: TreeGraph = {
  rootId: 1,
  persons: Object.fromEntries(names.map((n, i) => [String(i + 1), person(i + 1, n)])),
  families: [link(10, 2, 3, 1), link(11, 4, 5, 2), link(12, 6, 7, 3), link(13, 1, 8, 9)],
  truncated: false,
}

describe('tree rendering', () => {
  it('redraws only the cards whose selection or focus changed', async () => {
    mockApi({ 'GET /api/tree/1': [200, graph] })
    renderApp('/tree?root=1&view=hourglass&gen=3')
    const chart = await screen.findByRole('group', { name: 'Family tree chart' })
    const cards = await within(chart).findAllByRole('button', { name: /Weber/ })
    expect(cards.length).toBeGreaterThanOrEqual(8)

    vi.mocked(lifespan).mockClear()
    const karl = within(chart).getByRole('button', { name: /^Karl Weber/ })
    fireEvent.click(karl)
    await waitFor(() => expect(karl).toHaveAttribute('aria-pressed', 'true'))

    // The cards that lost and gained focus or selection re-render, not all of them.
    expect(vi.mocked(lifespan).mock.calls.length).toBeLessThanOrEqual(6)
    expect(vi.mocked(lifespan).mock.calls.length).toBeLessThan(cards.length - 2)
    expect(within(chart).getByRole('button', { name: /^Paul Weber/ })).toHaveAttribute('aria-pressed', 'false')
    const tabbable = within(chart)
      .getAllByRole('button')
      .filter((b) => b.getAttribute('tabindex') === '0')
    expect(tabbable.map((b) => b.dataset.treePerson)).toEqual(['4'])
  })
})
