import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { SourceDetail } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

const source: SourceDetail = {
  id: 7,
  title: 'Kirchenbuch St. Thomas',
  author: 'Ev.-luth. Gemeinde',
  publication: '',
  callNumber: 'KB 12',
  repository: { id: 3, name: 'Stadtarchiv Leipzig', address: '', url: '', notes: '' },
  notes: '',
  citationCount: 1,
  citations: [
    {
      id: 11,
      sourceId: 7,
      page: 'S. 12, Nr. 34',
      quality: 3,
      text: 'Anna, Tochter des Hans Müller',
      notes: '',
      links: [{ entityType: 'event', entityId: 5, field: '', status: 'accepted', statusReason: '', label: 'BIRT|Anna Müller', personId: 1 }],
    },
  ],
}

describe('sources', () => {
  it('lists sources with their repository and citation count', async () => {
    mockApi({ 'GET /api/sources': [200, [source]] })
    const { container } = renderApp('/sources')
    const link = await screen.findByRole('link', { name: /Kirchenbuch St. Thomas/ })
    expect(link).toHaveAttribute('href', '/sources/7')
    expect(link).toHaveTextContent('Stadtarchiv Leipzig · 1 citation')
    expect(await axeViolations(container)).toEqual([])
  })

  it('shows a source with what its citations support', async () => {
    mockApi({ 'GET /api/sources/7': [200, source] })
    const { container } = renderApp('/sources/7')
    expect(await screen.findByRole('heading', { level: 1, name: 'Kirchenbuch St. Thomas' })).toBeInTheDocument()
    expect(screen.getByText('KB 12')).toBeInTheDocument()
    expect(screen.getByText('(primary, direct evidence)')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Birth of Anna Müller' })).toHaveAttribute('href', '/people/1')
    expect(await axeViolations(container)).toEqual([])
  })

  it('cites a source when saving an event, and only then', async () => {
    let sent: Record<string, unknown> | undefined
    mockApi({
      'GET /api/persons/1': [
        200,
        {
          id: 1, givenNames: 'Anna', surname: 'Müller', namePrefix: '', nameSuffix: '', nickname: '', sex: 'F',
          isLiving: null, living: false, notes: '', alternateNames: [], citations: [], portrait: null, createdAt: '', updatedAt: '',
          events: [], parentFamilies: [], partnerFamilies: [],
        },
      ],
      'GET /api/sources': [200, [source]],
      'POST /api/events': (body) => {
        sent = body as Record<string, unknown>
        return [201, {}]
      },
    })
    const user = userEvent.setup()
    renderApp('/people/1')
    await screen.findByRole('heading', { level: 1, name: 'Anna Müller' })
    await user.keyboard('a')
    const dialog = await screen.findByRole('dialog', { name: 'New life event' })

    await user.type(within(dialog).getByRole('searchbox', { name: 'Source' }), 'kirch')
    await user.click(await within(dialog).findByRole('button', { name: /Kirchenbuch St. Thomas/ }))
    await user.type(within(dialog).getByLabelText('Where in the source'), 'S. 12')
    await user.selectOptions(within(dialog).getByLabelText('How reliable?'), 'primary, direct evidence')
    await user.click(within(dialog).getByRole('button', { name: '+ Cite this source' }))
    expect(within(dialog).getByText('new')).toBeInTheDocument()
    expect(sent).toBeUndefined()

    await user.click(within(dialog).getByRole('button', { name: /^Save/ }))
    await waitFor(() => expect(sent).toBeDefined())
    expect(sent!.addCitations).toEqual([{ sourceId: 7, page: 'S. 12', quality: 3, text: '' }])
  })
})

describe('repository field', () => {
  it('says so when creating a repository fails', async () => {
    mockApi({
      'GET /api/sources': [200, []],
      'GET /api/repositories': [200, []],
      'POST /api/repositories': [500, { error: 'disk full' }],
    })
    const user = userEvent.setup()
    renderApp('/sources')
    await user.click(await screen.findByRole('button', { name: '+ New source' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByRole('searchbox', { name: 'Kept at' }), 'Stadtarchiv')
    await user.click(await within(dialog).findByRole('button', { name: 'Create “Stadtarchiv”' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Could not save')
  })
})
