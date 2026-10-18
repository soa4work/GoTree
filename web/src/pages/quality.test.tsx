import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { CheckReport, DateProposals, PersonRef, RelationshipReport } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

const person = (id: number, givenNames: string, sex: PersonRef['sex'] = 'U'): PersonRef => ({
  id,
  givenNames,
  surname: 'Weber',
  sex,
  birthDate: '',
  deathDate: '',
  living: false,
  portrait: null,
})

const checks: CheckReport = {
  findings: [
    { origin: 'check:birth_after_death:1:0:0', rule: 'birth_after_death', severity: 'error', personId: 1 },
    { origin: 'check:parent_too_young:2:0:1', rule: 'parent_too_young', severity: 'warning', personId: 2, otherPersonId: 1, familyId: 5, years: 9 },
  ],
  persons: { 1: person(1, 'Anna', 'F'), 2: person(2, 'Paul', 'M') },
  taskIds: {},
}

const proposals: DateProposals = {
  items: [
    { eventId: 10, type: 'BIRT', customLabel: '', personId: 1, familyId: null, personIds: [1], raw: 'March 12, 1850', proposed: '12 MAR 1850', wasValid: false },
    { eventId: 11, type: 'MARR', customLabel: '', personId: null, familyId: 5, personIds: [1, 2], raw: 'ca. 1870', proposed: 'ABT 1870', wasValid: false },
  ],
  unreadable: 1,
  persons: { 1: person(1, 'Anna'), 2: person(2, 'Paul') },
}

describe('data quality page', () => {
  it('lists findings and rewrites the dates left selected', async () => {
    let sent: unknown
    mockApi({
      'GET /api/checks': [200, checks],
      'GET /api/dates/proposals': [200, proposals],
      'POST /api/dates/normalize': (body) => {
        sent = body
        return [200, { updated: 1, skipped: 0 }]
      },
    })
    const { container } = renderApp('/quality')

    expect(await screen.findByText('Anna Weber was born after they died.')).toBeInTheDocument()
    expect(screen.getByText('Paul Weber was born when Anna Weber was at most 9.')).toBeInTheDocument()
    expect(screen.getByText('2 possible problems')).toHaveAttribute('role', 'status')

    fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'parent_too_young' } })
    expect(screen.queryByText('Anna Weber was born after they died.')).not.toBeInTheDocument()

    const marriage = await screen.findByRole('checkbox', { name: /ca\. 1870 becomes ABT 1870/ })
    expect(screen.getByRole('checkbox', { name: /March 12, 1850 becomes 12 MAR 1850/ })).toBeChecked()
    expect(screen.getByText(/family of Anna Weber & Paul Weber/)).toBeInTheDocument()
    expect(screen.getByText(/1 date cannot be read at all/)).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])

    fireEvent.click(marriage)
    fireEvent.click(screen.getByRole('button', { name: 'Rewrite 1 date' }))
    expect(await screen.findByText('1 date rewritten.')).toBeInTheDocument()
    expect(sent).toEqual({ changes: [{ eventId: 10, raw: 'March 12, 1850', date: '12 MAR 1850' }] })
  })
})

describe('relationship page', () => {
  it('names the relationship and shows the path', async () => {
    const rep: RelationshipReport = {
      a: 1,
      b: 4,
      kind: 'blood',
      kinship: { up: 1, down: 2, half: false, adoptive: false, ancestors: [-7], path: [1, -7, 3, 4] },
      others: [],
      persons: { 1: person(1, 'Anna', 'F'), 3: person(3, 'Berta', 'F'), 4: person(4, 'Clara', 'F') },
    }
    mockApi({
      'GET /api/persons/1': [200, { ...person(1, 'Anna', 'F'), events: [] }],
      'GET /api/persons/4': [200, { ...person(4, 'Clara', 'F'), events: [] }],
      'GET /api/relationship': [200, rep],
    })
    const { container } = renderApp('/relationship?a=1&b=4')

    expect(await screen.findByText('Clara Weber is Anna Weber’s niece.')).toBeInTheDocument()
    const steps = within(screen.getByRole('region', { name: 'How they are connected' })).getAllByRole('listitem')
    expect(steps.map((s) => s.textContent)).toEqual([
      'Anna Weber',
      'parent of the previousunknown parents',
      'child of the previousBerta Weber',
      'child of the previousClara Weber',
    ])
    await waitFor(async () => expect(await axeViolations(container)).toEqual([]))
  })
})

describe('making a task from a finding', () => {
  it('says so when saving the task fails', async () => {
    mockApi({
      'GET /api/checks': [200, checks],
      'GET /api/dates/proposals': [200, { items: [], unreadable: 0, persons: {} }],
      'POST /api/research/tasks': [500, { error: 'disk full' }],
    })
    renderApp('/quality')
    await screen.findByText('Anna Weber was born after they died.')
    fireEvent.click(screen.getAllByRole('button', { name: /Make a task/ })[0]!)
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not save')
  })
})
