import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { LogEntry, ResearchTask } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

const task: ResearchTask = {
  id: 7,
  title: 'Find the baptism of Anna Weber',
  status: 'open',
  priority: 'high',
  dueOn: '2020-01-01',
  notes: '',
  origin: '',
  doneAt: null,
  links: [{ entityType: 'person', entityId: 1, label: 'Anna Weber' }],
  logCount: 1,
  createdAt: '',
  updatedAt: '',
}

const entry: LogEntry = {
  id: 3,
  taskId: 7,
  taskTitle: task.title,
  searchedOn: '2026-10-01',
  query: 'Baptisms 1850–1855',
  location: 'Stadtarchiv Leipzig',
  result: 'not_found',
  notes: '',
  links: [],
  createdAt: '',
  updatedAt: '',
}

describe('research page', () => {
  it('lists tasks and logs a search for one', async () => {
    let sent: unknown
    mockApi({
      'GET /api/research/tasks': [200, [task]],
      'GET /api/research/log': [200, [entry]],
      'POST /api/research/log': (body) => {
        sent = body
        return [201, { ...entry, id: 4 }]
      },
    })
    const { container } = renderApp('/research')

    const todo = await screen.findByRole('region', { name: 'To do' })
    const item = await within(todo).findByRole('button', { name: /Find the baptism of Anna Weber/ })
    expect(item).toHaveTextContent('Open · High · overdue since')
    expect(item).toHaveTextContent('1 search · Anna Weber')
    expect(screen.getByRole('button', { name: /Not found\s*Baptisms 1850–1855/ })).toHaveTextContent(
      'Stadtarchiv Leipzig · for: Find the baptism',
    )
    expect(await axeViolations(container)).toEqual([])

    fireEvent.click(screen.getByRole('button', { name: '+ Log a search' }))
    const dialog = screen.getByRole('dialog', { name: 'Log a search' })
    fireEvent.change(within(dialog).getByLabelText(/^What you searched for/), { target: { value: 'Deaths 1900–1910' } })
    expect(within(dialog).getByLabelText('Not found')).toBeChecked()
    fireEvent.click(within(dialog).getByLabelText('Found'))
    await waitFor(() => expect(within(dialog).getByRole('option', { name: task.title })).toBeInTheDocument())
    fireEvent.change(within(dialog).getByLabelText('Task (optional)'), { target: { value: '7' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /^Save/ }))

    await waitFor(() =>
      expect(sent).toMatchObject({ taskId: 7, query: 'Deaths 1900–1910', result: 'found', links: [] }),
    )
  })
})

describe('person page research', () => {
  it('turns a missing fact into a task', async () => {
    let sent: unknown
    mockApi({
      'GET /api/persons/1': [
        200,
        {
          id: 1,
          givenNames: 'Anna',
          surname: 'Weber',
          namePrefix: '',
          nameSuffix: '',
          nickname: '',
          sex: 'F',
          isLiving: null,
          living: false,
          notes: '',
          alternateNames: [],
          citations: [],
          portrait: null,
          createdAt: '',
          updatedAt: '',
          events: [],
          parentFamilies: [],
          partnerFamilies: [],
        },
      ],
      'GET /api/research/suggestions': [
        200,
        { suggestions: [{ origin: 'missing:missing_birth:1', kind: 'missing_birth', personId: 1 }], persons: {} },
      ],
      'GET /api/research/tasks': [200, []],
      'GET /api/research/log': [200, []],
      'GET /api/checks': [200, { findings: [], persons: {}, taskIds: {} }],
      'GET /api/media': [200, []],
      'POST /api/research/tasks': (body) => {
        sent = body
        return [201, { ...task, id: 8 }]
      },
    })
    renderApp('/people/1')

    const region = await screen.findByRole('region', { name: 'Worth researching' })
    expect(within(region).getByText('Find a record of Anna Weber’s birth or baptism')).toBeInTheDocument()
    fireEvent.click(within(region).getByRole('button', { name: /Make a task/ }))
    await waitFor(() =>
      expect(sent).toEqual({
        title: 'Find a record of Anna Weber’s birth or baptism',
        status: 'open',
        priority: 'normal',
        dueOn: '',
        notes: '',
        origin: 'missing:missing_birth:1',
        links: [{ entityType: 'person', entityId: 1 }],
      }),
    )
  })
})

describe('person page research errors', () => {
  it('says so when saving a suggested task fails', async () => {
    mockApi({
      'GET /api/persons/1': [
        200,
        {
          id: 1,
          givenNames: 'Anna',
          surname: 'Weber',
          namePrefix: '',
          nameSuffix: '',
          nickname: '',
          sex: 'F',
          isLiving: null,
          living: false,
          notes: '',
          alternateNames: [],
          citations: [],
          portrait: null,
          createdAt: '',
          updatedAt: '',
          events: [],
          parentFamilies: [],
          partnerFamilies: [],
        },
      ],
      'GET /api/research/suggestions': [
        200,
        { suggestions: [{ origin: 'missing:missing_birth:1', kind: 'missing_birth', personId: 1 }], persons: {} },
      ],
      'GET /api/research/tasks': [200, []],
      'GET /api/research/log': [200, []],
      'GET /api/checks': [200, { findings: [], persons: {}, taskIds: {} }],
      'GET /api/media': [200, []],
      'POST /api/research/tasks': [500, { error: 'disk full' }],
    })
    renderApp('/people/1')

    const region = await screen.findByRole('region', { name: 'Worth researching' })
    fireEvent.click(within(region).getByRole('button', { name: /Make a task/ }))
    expect(await within(region).findByRole('alert')).toHaveTextContent('Could not save')
  })
})
