import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PersonRef, TreeGraph } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'
import { elbow } from './geometry'
import { fitChart, MARGIN_MM, papers, TITLE_MM } from './paper'

describe('connector paths', () => {
  it('rounds the corners of an elbow and keeps straight lines straight', () => {
    expect(elbow(0, 0, 100, 40)).toBe('M 0 0 V 10 Q 0 20 10 20 H 90 Q 100 20 100 30 V 40')
    expect(elbow(50, 0, 50, 40)).toBe('M 50 0 V 20 H 50 V 40')
  })
})

describe('fitting a chart to paper', () => {
  it('picks the orientation that prints larger and centres the chart', () => {
    const wide = fitChart(4000, 1000, papers.A3, 'auto', true)
    expect(wide.landscape).toBe(true)
    expect([wide.paperW, wide.paperH]).toEqual([420, 297])
    expect(wide.scale).toBeCloseTo((420 - 2 * MARGIN_MM) / 4000)
    expect(wide.x).toBeCloseTo(MARGIN_MM)
    expect(wide.y).toBeGreaterThan(MARGIN_MM + TITLE_MM)

    const forced = fitChart(4000, 1000, papers.A3, 'portrait', false)
    expect(forced.landscape).toBe(false)
    expect(forced.namePt).toBeLessThan(wide.namePt)
  })

  it('does not blow a small tree up', () => {
    const small = fitChart(200, 100, papers.A0, 'auto', false)
    expect(small.namePt).toBeLessThan(20)
  })
})

const person = (id: number, givenNames: string, extra: Partial<PersonRef> = {}): PersonRef => ({
  id,
  givenNames,
  surname: 'Weber',
  sex: 'U',
  birthDate: '',
  deathDate: '',
  living: false,
  portrait: null,
  ...extra,
})

const graph: TreeGraph = {
  rootId: 1,
  persons: { 1: person(1, 'Paul', { sex: 'M', birthDate: '1890' }), 2: person(2, 'Hans', { sex: 'M' }), 3: person(3, 'Maria', { sex: 'F' }) },
  families: [{ id: 10, partner1Id: 2, partner2Id: 3, unionType: 'married', children: [{ personId: 1, relationPartner1: 'birth', relationPartner2: 'birth' }] }],
  truncated: false,
}

describe('chart page', () => {
  afterEach(() => vi.restoreAllMocks())

  it('draws the chart on the chosen paper', async () => {
    mockApi({
      'GET /api/tree/1': [200, graph],
      'GET /api/persons/1': [200, { ...graph.persons[1], events: [], parentFamilies: [], partnerFamilies: [] }],
    })
    const { container } = renderApp('/chart?root=1')
    const chart = await screen.findByRole('img', { name: 'Ancestors of Paul Weber: chart of 3 people' })
    expect(within(chart).getByText('♂ Paul')).toBeInTheDocument()
    expect(within(chart).getAllByText('Weber')).not.toHaveLength(0)
    expect(chart).toHaveTextContent('* 1890')
    expect(screen.getByText(/3 people on A3/)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Paper'), { target: { value: 'A4' } })
    fireEvent.change(screen.getByLabelText('Orientation'), { target: { value: 'landscape' } })
    expect(chart.getAttribute('width')).toBe('297mm')
    expect(container.querySelector('style')?.textContent).toContain('size: 297mm 210mm')
    expect(await axeViolations(container)).toEqual([])
  })

  it('fetches at most four photos at a time when saving the SVG', async () => {
    const withPhoto = (id: number, name: string) =>
      person(id, name, { portrait: { mediaId: 100 + id, regionId: null } as PersonRef['portrait'] })
    const link = (id: number, p1: number, p2: number, child: number) => ({
      id,
      partner1Id: p1,
      partner2Id: p2,
      unionType: 'married' as const,
      children: [{ personId: child, relationPartner1: 'birth' as const, relationPartner2: 'birth' as const }],
    })
    // Seven people with a photo each: more than the limit of four.
    const big: TreeGraph = {
      rootId: 1,
      persons: Object.fromEntries(
        ['Paul', 'Hans', 'Maria', 'Karl', 'Anna', 'Otto', 'Ida'].map((n, i) => [i + 1, withPhoto(i + 1, n)]),
      ),
      families: [link(10, 2, 3, 1), link(11, 4, 5, 2), link(12, 6, 7, 3)],
      truncated: false,
    }
    const base = mockApi({
      'GET /api/tree/1': [200, big],
      'GET /api/persons/1': [200, { ...big.persons[1], events: [], parentFamilies: [], partnerFamilies: [] }],
    })
    const pending: Array<() => void> = []
    const requested: string[] = []
    let inFlight = 0
    let peak = 0
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (!url.includes('/thumb')) return base(input, init)
        requested.push(url)
        inFlight++
        peak = Math.max(peak, inFlight)
        return new Promise<Response>((resolve) => {
          pending.push(() => {
            inFlight--
            resolve({ ok: true, status: 200, blob: () => Promise.resolve(new Blob(['x'])) } as Response)
          })
        })
      }),
    )
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:x'), revokeObjectURL: vi.fn() })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    renderApp('/chart?root=1')
    await screen.findByRole('img', { name: /chart of 7 people/ })
    const button = screen.getByRole('button', { name: 'Download SVG' })
    fireEvent.click(button)
    expect(button).toBeDisabled()
    fireEvent.click(button)

    await waitFor(() => expect(pending.length).toBe(4))
    while (requested.length < 7 || pending.length > 0) {
      pending.shift()?.()
      await new Promise((r) => setTimeout(r, 0))
    }
    await waitFor(() => expect(click).toHaveBeenCalledTimes(1))
    expect(peak).toBeLessThanOrEqual(4)
    expect(new Set(requested).size).toBe(7)
    expect(requested).toHaveLength(7)
    await waitFor(() => expect(button).toBeEnabled())
  })
})
