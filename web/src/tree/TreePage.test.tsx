import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { PersonRef, TreeGraph } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

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
  persons: {
    '1': person(1, 'Paul', { sex: 'M', birthDate: '1890' }),
    '2': person(2, 'Hans', { sex: 'M' }),
    '3': person(3, 'Maria', { sex: 'F' }),
    '4': person(4, 'Eva', { sex: 'F' }),
    '5': person(5, 'Mia'),
  },
  families: [
    { id: 10, partner1Id: 2, partner2Id: 3, unionType: 'married', children: [{ personId: 1, relationPartner1: 'birth', relationPartner2: 'birth' }] },
    { id: 11, partner1Id: 1, partner2Id: 4, unionType: 'married', children: [{ personId: 5, relationPartner1: 'birth', relationPartner2: 'birth' }] },
  ],
  truncated: false,
}

describe('tree page', () => {
  beforeEach(() => localStorage.clear())

  it('draws the chart with one tab stop and selects a person', async () => {
    const fetchMock = mockApi({ 'GET /api/tree/1': [200, graph] })
    renderApp('/tree?root=1&view=hourglass&gen=2')

    expect(await screen.findByRole('heading', { level: 1, name: 'Tree of Paul Weber' })).toBeInTheDocument()
    const chart = await screen.findByRole('group', { name: 'Family tree chart' })
    const paul = await within(chart).findByRole('button', { name: /^Paul Weber/ })
    expect(paul).toHaveAttribute('aria-current', 'true')
    expect(paul).toHaveAccessibleName(expect.stringContaining('Male'))
    // Only the root is in the tab order; arrow keys reach the others.
    const tabbable = within(chart).getAllByRole('button').filter((b) => b.getAttribute('tabindex') === '0')
    expect(tabbable.map((b) => b.dataset.treePerson)).toEqual(['1'])

    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/tree/1?up=2&down=2'))).toBe(true)
    expect(screen.getByText('5 people shown.')).toHaveAttribute('role', 'status')

    // fireEvent instead of userEvent: a pointer-down would reach d3-zoom,
    // which needs event.view, and jsdom leaves it null. Real clicks are
    // covered by the Playwright tests.
    fireEvent.click(within(chart).getByRole('button', { name: /^Eva Weber/ }))
    await waitFor(() => expect(within(chart).getByRole('button', { name: /^Eva Weber/ })).toHaveAttribute('aria-pressed', 'true'))
    const panel = await screen.findByRole('region', { name: 'Eva Weber' })
    expect(within(panel).getByRole('button', { name: 'Center tree here' })).toBeInTheDocument()
    expect(within(panel).getByRole('link', { name: 'Open page' })).toHaveAttribute('href', '/people/4')
  })

  it('shows the same tree as nested lists', async () => {
    mockApi({ 'GET /api/tree/1': [200, graph] })
    const { container } = renderApp('/tree?root=1&view=hourglass&gen=2&mode=list')

    const ancestors = await screen.findByRole('region', { name: 'Ancestors' })
    const parents = within(ancestors).getByRole('list', { name: 'Parents of Paul Weber' })
    expect(within(parents).getByRole('link', { name: 'Hans Weber' })).toHaveAttribute('href', '/people/2')
    const descendants = screen.getByRole('region', { name: 'Descendants' })
    expect(within(descendants).getByRole('link', { name: 'Eva Weber' })).toBeInTheDocument()
    const children = within(descendants).getByRole('list', { name: 'Children of Paul Weber' })
    expect(within(children).getByRole('link', { name: 'Mia Weber' })).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })

  it('hides partners with blood relatives only', async () => {
    mockApi({ 'GET /api/tree/1': [200, graph] })
    renderApp('/tree?root=1&view=descendants&mode=list&blood=1')
    const descendants = await screen.findByRole('region', { name: 'Descendants' })
    expect(within(descendants).queryByRole('link', { name: 'Eva Weber' })).toBeNull()
    expect(within(descendants).getByRole('link', { name: 'Mia Weber' })).toBeInTheDocument()
  })

  it('starts from the first person when there is no root', async () => {
    localStorage.clear()
    mockApi({
      'GET /api/persons': [200, { items: [graph.persons['1']], total: 1 }],
      'GET /api/tree/1': [200, graph],
    })
    renderApp('/tree')
    expect(await screen.findByRole('heading', { level: 1, name: 'Tree of Paul Weber' })).toBeInTheDocument()
  })
})

describe('tree page root', () => {
  beforeEach(() => localStorage.clear())

  it('forgets a remembered root that is gone', async () => {
    localStorage.setItem('gotree.treeRoot.1', '99')
    mockApi({
      'GET /api/tree/99': [404, { error: 'not found' }],
      'GET /api/persons': [200, { items: [graph.persons['1']], total: 1 }],
      'GET /api/tree/1': [200, graph],
    })
    renderApp('/tree')
    expect(await screen.findByRole('heading', { level: 1, name: 'Tree of Paul Weber' })).toBeInTheDocument()
    await waitFor(() => expect(localStorage.getItem('gotree.treeRoot.1')).toBe('1'))
    expect(screen.queryByText('Could not load the data. Please try again.')).toBeNull()
  })

  it('shows onboarding after the tree was emptied', async () => {
    localStorage.setItem('gotree.treeRoot.1', '99')
    mockApi({
      'GET /api/tree/99': [404, { error: 'not found' }],
      'GET /api/persons': [200, { items: [], total: 0 }],
    })
    renderApp('/tree')
    await waitFor(() => expect(localStorage.getItem('gotree.treeRoot.1')).toBeNull())
    expect(await screen.findByRole('heading', { name: 'No one is in your tree yet' })).toBeInTheDocument()
    expect(screen.queryByRole('group', { name: 'Family tree chart' })).toBeNull()
  })

  it('explains a root from the URL that is gone', async () => {
    mockApi({ 'GET /api/tree/99': [404, { error: 'not found' }] })
    renderApp('/tree?root=99')
    expect(await screen.findByRole('alert')).toHaveTextContent('This person is no longer in the tree.')
    expect(screen.getByRole('link', { name: 'Show the tree' })).toHaveAttribute('href', '/tree')
  })

  it('opens the last centered person when the Tree link is followed again', async () => {
    localStorage.setItem('gotree.treeRoot.1', '1')
    mockApi({
      'GET /api/tree/1': [200, graph],
      'GET /api/tree/2': [200, { ...graph, rootId: 2 }],
    })
    renderApp('/tree')
    const chart = await screen.findByRole('group', { name: 'Family tree chart' })
    fireEvent.doubleClick(within(chart).getByRole('button', { name: /^Hans Weber/ }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Tree of Hans Weber' })).toBeInTheDocument()
    await waitFor(() => expect(localStorage.getItem('gotree.treeRoot.1')).toBe('2'))

    fireEvent.click(screen.getAllByRole('link', { name: 'Tree' })[0]!)
    expect(await screen.findByRole('heading', { level: 1, name: 'Tree of Hans Weber' })).toBeInTheDocument()
    expect(localStorage.getItem('gotree.treeRoot.1')).toBe('2')
  })

  it('keys the remembered root by tree', async () => {
    localStorage.setItem('gotree.treeRoot.2', '5')
    const fetchMock = mockApi({
      'GET /api/persons': [200, { items: [graph.persons['1']], total: 1 }],
      'GET /api/tree/1': [200, graph],
    })
    renderApp('/tree')
    expect(await screen.findByRole('heading', { level: 1, name: 'Tree of Paul Weber' })).toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/tree/5'))).toBe(false)
    expect(localStorage.getItem('gotree.treeRoot.2')).toBe('5')
  })
})

describe('tree page on narrow screens', () => {
  it('collapses the controls behind an Options button', async () => {
    mockApi({ 'GET /api/tree/1': [200, graph] })
    renderApp('/tree?root=1&view=hourglass&gen=2')
    const options = await screen.findByRole('button', { name: /^Options/ })
    expect(options).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(options)
    expect(options).toHaveAttribute('aria-expanded', 'true')
    expect(document.getElementById(options.getAttribute('aria-controls')!)).toHaveClass('grid')
  })

  it('shows the selected person as a sheet that Escape closes', async () => {
    mockApi({ 'GET /api/tree/1': [200, graph] })
    renderApp('/tree?root=1&view=hourglass&gen=2&sel=4')
    const sheet = await screen.findByRole('region', { name: 'Eva Weber' })
    expect(sheet).toHaveClass('absolute')
    await waitFor(() => expect(sheet).toHaveFocus())
    fireEvent.keyDown(sheet, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Eva Weber' })).toBeNull())
  })

  it('keeps the sheet open when Escape is pressed inside the add-relative dialog', async () => {
    mockApi({
      'GET /api/tree/1': [200, graph],
      'GET /api/persons/4': [
        200,
        {
          ...graph.persons['4'],
          namePrefix: '',
          nameSuffix: '',
          nickname: '',
          isLiving: null,
          notes: '',
          alternateNames: [],
          citations: [],
          createdAt: '',
          updatedAt: '',
          events: [],
          parentFamilies: [],
          partnerFamilies: [],
        },
      ],
    })
    renderApp('/tree?root=1&view=hourglass&gen=2&sel=4')
    const sheet = await screen.findByRole('region', { name: 'Eva Weber' })
    const add = within(sheet).getByRole('button', { name: '+ Parent' })
    await waitFor(() => expect(add).toBeEnabled())
    fireEvent.click(add)
    const field = await screen.findByLabelText('Given names')
    fireEvent.change(field, { target: { value: 'Ida' } })
    fireEvent.keyDown(field, { key: 'Escape' })
    expect(screen.getByRole('region', { name: 'Eva Weber' })).toBeInTheDocument()
    expect(screen.getByLabelText('Given names')).toHaveValue('Ida')
  })
})
