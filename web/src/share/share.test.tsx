import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import type { PersonRef, ShareLink } from '../api/types'
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

const link: ShareLink = {
  id: 1,
  label: 'Cousins',
  scope: 'descendants',
  root: person(2, 'Hans'),
  privacy: 'living_names',
  expiresAt: null,
  revokedAt: null,
  lastUsedAt: null,
  createdAt: '2026-10-06T08:00:00Z',
}

function setClipboard(clipboard: unknown) {
  Object.defineProperty(navigator, 'clipboard', { value: clipboard, configurable: true })
}

afterEach(() => {
  Reflect.deleteProperty(navigator, 'clipboard')
})

describe('sharing page', () => {
  it('creates a link and shows it once', async () => {
    let sent: unknown
    mockApi({
      'GET /api/share-links': [200, [link]],
      'GET /api/calendar-feeds': [200, []],
      'POST /api/share-links': (body) => {
        sent = body
        return [
          201,
          { ...link, id: 2, label: 'Aunt Erna', scope: 'tree', root: null, privacy: 'deceased', token: 'tok123' },
        ]
      },
    })
    const { container } = renderApp('/sharing')

    expect(await screen.findByText('Cousins')).toBeInTheDocument()
    expect(
      screen.getByText(/Descendants of Hans Weber · Show living people by name only · no end date · not used yet/),
    ).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])

    fireEvent.click(screen.getByRole('button', { name: '+ New share link' }))
    const dialog = screen.getByRole('dialog', { name: 'New share link' })
    fireEvent.change(within(dialog).getByLabelText(/^Name/), { target: { value: 'Aunt Erna' } })
    expect(within(dialog).getByLabelText('The whole tree')).toBeChecked()
    expect(within(dialog).getByLabelText('Leave living people out')).toBeChecked()
    fireEvent.click(within(dialog).getByRole('button', { name: /Create link/ }))

    expect(await screen.findByRole('heading', { name: 'Share link “Aunt Erna” is ready' })).toBeInTheDocument()
    expect(screen.getByLabelText('Share link')).toHaveValue(`${window.location.origin}/share/tok123`)
    expect(sent).toEqual({ label: 'Aunt Erna', scope: 'tree', rootPersonId: null, privacy: 'deceased', expiresOn: '' })
  })

  it('re-issues an address', async () => {
    mockApi({
      'GET /api/share-links': [200, [link]],
      'GET /api/calendar-feeds': [200, []],
      'POST /api/share-links/1/token': [200, { ...link, token: 'abc123' }],
    })
    renderApp('/sharing')

    fireEvent.click(await screen.findByRole('button', { name: 'New address: Cousins' }))
    const dialog = screen.getByRole('dialog', { name: 'Make a new address?' })
    expect(within(dialog).getByText(/current address of “Cousins” will stop working/)).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: 'New address' }))

    expect(await screen.findByRole('heading', { name: 'New address for “Cousins”' })).toBeInTheDocument()
    expect(screen.getByLabelText('Share link')).toHaveValue(`${window.location.origin}/share/abc123`)
  })

  it('says when copying fails', async () => {
    mockApi({
      'GET /api/share-links': [200, []],
      'GET /api/calendar-feeds': [200, []],
      'POST /api/share-links': [201, { ...link, id: 2, token: 'tok123' }],
    })
    setClipboard({ writeText: () => Promise.reject(new Error('denied')) })
    renderApp('/sharing')
    fireEvent.click(await screen.findByRole('button', { name: '+ New share link' }))
    const dialog = screen.getByRole('dialog', { name: 'New share link' })
    fireEvent.change(within(dialog).getByLabelText(/^Name/), { target: { value: 'Aunt Erna' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /Create link/ }))
    await screen.findByLabelText('Share link')

    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not copy. Select the address and copy it by hand.')

    // Over plain http there is no clipboard at all.
    setClipboard(undefined)
    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not copy')

    const writeText = vi.fn(() => Promise.resolve())
    setClipboard({ writeText })
    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    await waitFor(() => expect(screen.getByText('Copied.')).toBeInTheDocument())
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/share/tok123`)
  })
})

describe('calendar feeds', () => {
  it('creates a calendar address with living people', async () => {
    let sent: unknown
    mockApi({
      'GET /api/share-links': [200, []],
      'GET /api/calendar-feeds': [200, []],
      'POST /api/calendar-feeds': (body) => {
        sent = body
        return [201, { id: 1, label: 'My phone', includeLiving: true, revokedAt: null, lastUsedAt: null, createdAt: '', token: 'cal1' }]
      },
    })
    const { container } = renderApp('/sharing')
    fireEvent.click(await screen.findByRole('button', { name: '+ New calendar address' }))
    const dialog = screen.getByRole('dialog', { name: 'New calendar address' })
    fireEvent.change(within(dialog).getByLabelText(/^Name/), { target: { value: 'My phone' } })
    fireEvent.click(within(dialog).getByLabelText('Include living people'))
    fireEvent.click(within(dialog).getByRole('button', { name: /Create address/ }))

    expect(await screen.findByLabelText('Calendar address')).toHaveValue(`${window.location.origin}/ical/cal1.ics?lang=en`)
    expect(screen.getByRole('link', { name: 'Open in calendar app' }).getAttribute('href')).toMatch(/^webcal:\/\/.*\/ical\/cal1\.ics\?lang=en$/)
    expect(sent).toEqual({ label: 'My phone', includeLiving: true })
    expect(await axeViolations(container)).toEqual([])
  })

  it.each(['ru', 'kk'])('puts the interface language %s into the calendar address', async (lang) => {
    await i18n.changeLanguage(lang)
    try {
      mockApi({
        'GET /api/share-links': [200, []],
        'GET /api/calendar-feeds': [200, []],
        'POST /api/calendar-feeds': () => [
          201,
          { id: 1, label: 'Phone', includeLiving: false, revokedAt: null, lastUsedAt: null, createdAt: '', token: 'cal1' },
        ],
      })
      renderApp('/sharing')
      fireEvent.click(await screen.findByRole('button', { name: `+ ${i18n.t('calendar.new')}` }))
      const dialog = screen.getByRole('dialog')
      fireEvent.change(within(dialog).getAllByRole('textbox').at(0)!, { target: { value: 'Phone' } })
      fireEvent.click(dialog.querySelector('button[type="submit"]')!)
      await waitFor(() => expect(document.querySelector('input[readonly]')).not.toBeNull())
      expect(document.querySelector('input[readonly]')).toHaveValue(`${window.location.origin}/ical/cal1.ics?lang=${lang}`)
      expect(screen.getByText(new RegExp(`lang=${lang}`))).toBeInTheDocument()
    } finally {
      await i18n.changeLanguage('en')
    }
  })

  it('says when copying a calendar address fails', async () => {
    mockApi({
      'GET /api/share-links': [200, []],
      'GET /api/calendar-feeds': [200, []],
      'POST /api/calendar-feeds': [
        201,
        { id: 1, label: 'My phone', includeLiving: false, revokedAt: null, lastUsedAt: null, createdAt: '', token: 'cal1' },
      ],
    })
    setClipboard(undefined)
    renderApp('/sharing')
    fireEvent.click(await screen.findByRole('button', { name: '+ New calendar address' }))
    const dialog = screen.getByRole('dialog', { name: 'New calendar address' })
    fireEvent.change(within(dialog).getByLabelText(/^Name/), { target: { value: 'My phone' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /Create address/ }))
    await screen.findByLabelText('Calendar address', { exact: true })

    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not copy. Select the address and copy it by hand.')
  })
})

describe('share view', () => {
  it('shows a person read-only without an account', async () => {
    const fetchMock = mockApi({
      'GET /api/auth/state': [200, { setupRequired: false, user: null, tree: null }],
      'GET /api/share/tok/persons/2': [
        200,
        {
          ...person(2, 'Hans'),
          namePrefix: '',
          nameSuffix: '',
          nickname: '',
          isLiving: null,
          notes: 'Weaver in Leipzig',
          alternateNames: [],
          citations: [],
          createdAt: '',
          updatedAt: '',
          events: [],
          parentFamilies: [],
          partnerFamilies: [
            {
              id: 5,
              partner1: person(2, 'Hans'),
              partner2: null,
              unionType: 'married',
              notes: '',
              children: [
                {
                  person: person(3, 'Lisa', { living: true }),
                  relationPartner1: 'birth',
                  relationPartner2: 'birth',
                  citations: [],
                },
              ],
              events: [],
              citations: [],
              createdAt: '',
              updatedAt: '',
            },
          ],
        },
      ],
      'GET /api/share/tok': [
        200,
        { treeName: 'Weber family', label: 'Cousins', scope: 'tree', privacy: 'living_names', root: null, startId: 2 },
      ],
    })
    const { container } = renderApp('/share/tok/people/2')

    expect(await screen.findByRole('heading', { level: 1, name: 'Hans Weber' })).toBeInTheDocument()
    expect(screen.getByText('Shared from Weber family')).toBeInTheDocument()
    expect(screen.getByText('Read-only view')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Lisa Weber' })).toHaveAttribute('href', '/share/tok/people/3')
    expect(screen.queryByRole('button', { name: /Edit/ })).not.toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/auth/state'))).toBe(false)
    expect(await axeViolations(container)).toEqual([])
  })

  it('explains a link that no longer works', async () => {
    mockApi({})
    renderApp('/share/gone')
    expect(await screen.findByRole('heading', { name: 'This link does not work' })).toBeInTheDocument()
    expect(screen.getByText(/expired or been withdrawn/)).toBeInTheDocument()
  })
})
