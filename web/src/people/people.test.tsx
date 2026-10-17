import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Family, LifeEvent, PersonDetail, PersonRef } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

const ref = (id: number, givenNames: string, extra: Partial<PersonRef> = {}): PersonRef => ({
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

const event = (id: number, type: string, raw: string, extra: Partial<LifeEvent> = {}): LifeEvent => ({
  id,
  personId: 1,
  familyId: null,
  type,
  customLabel: '',
  date: { raw, normalized: raw, qualifier: '', valid: true, sortKey: Number(raw.slice(-4)) * 10000 },
  place: null,
  description: '',
  notes: '',
  status: 'accepted',
  statusReason: '',
  sortOrder: 0,
  participants: [],
  citations: [],
  createdAt: '',
  updatedAt: '',
  ...extra,
})

const family = (id: number, extra: Partial<Family>): Family => ({
  id,
  partner1: null,
  partner2: null,
  unionType: 'married',
  notes: '',
  children: [],
  events: [],
  citations: [],
  createdAt: '',
  updatedAt: '',
  ...extra,
})

const paul: PersonDetail = {
  id: 1,
  givenNames: 'Paul',
  surname: 'Weber',
  namePrefix: '',
  nameSuffix: '',
  nickname: '',
  sex: 'M',
  isLiving: null,
  living: false,
  notes: 'Worked as a weaver.',
  alternateNames: [],
  citations: [],
  portrait: null,
  createdAt: '',
  updatedAt: '',
  events: [
    event(10, 'DEAT', '1950', { status: 'disputed', statusReason: 'Only family tradition' }),
    event(11, 'BIRT', '12 MAR 1890'),
    event(12, 'CENS', '1900', { personId: 2, role: 'child' }),
  ],
  parentFamilies: [
    family(20, {
      partner1: ref(2, 'Hans'),
      partner2: null,
      children: [
        { person: ref(1, 'Paul'), relationPartner1: 'birth', relationPartner2: 'adopted', sortOrder: 0 },
        { person: ref(3, 'Lena'), relationPartner1: 'birth', relationPartner2: 'birth', sortOrder: 1 },
      ],
    }),
  ],
  partnerFamilies: [
    family(21, {
      partner1: ref(1, 'Paul'),
      partner2: ref(4, 'Eva'),
      events: [event(13, 'MARR', '1915', { personId: null, familyId: 21 })],
      children: [{ person: ref(5, 'Mia', { birthDate: '1920' }), relationPartner1: 'birth', relationPartner2: 'birth', sortOrder: 0 }],
    }),
  ],
}

describe('people list', () => {
  it('searches and opens the new-person dialog', async () => {
    const fetchMock = mockApi({
      'GET /api/persons': [200, { items: [ref(1, 'Paul', { birthDate: '1890', deathDate: '1950' })], total: 1 }],
    })
    const user = userEvent.setup()
    const { container } = renderApp('/people')

    expect(await screen.findByRole('link', { name: /Paul Weber/ })).toHaveAttribute('href', '/people/1')
    expect(screen.getByText('1890 – 1950')).toBeInTheDocument()
    expect(screen.getByText('1 person')).toHaveAttribute('role', 'status')
    expect(await axeViolations(container)).toEqual([])

    await user.type(screen.getByRole('searchbox', { name: 'Search people' }), 'webr')
    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([url]) => String(url).includes('q=webr'))).toBe(true),
    )

    await user.click(screen.getByRole('button', { name: /New person/ }))
    expect(await screen.findByRole('dialog', { name: 'New person' })).toBeInTheDocument()
    expect(screen.getByLabelText('Given names')).toHaveFocus()
  })

  it('opens the new-person dialog with n', async () => {
    mockApi({ 'GET /api/persons': [200, { items: [], total: 0 }] })
    const user = userEvent.setup()
    renderApp('/people')
    await screen.findByText(/No one is in your tree yet/)
    await user.keyboard('n')
    expect(await screen.findByRole('dialog', { name: 'New person' })).toBeInTheDocument()
  })
})

describe('person page', () => {
  it('shows events in date order, families and relations', async () => {
    mockApi({ 'GET /api/persons/1': [200, paul] })
    const { container } = renderApp('/people/1')

    expect(await screen.findByRole('heading', { level: 1, name: 'Paul Weber' })).toBeInTheDocument()
    expect(screen.getByText(/Male · 1890 – 1950 · deceased/)).toBeInTheDocument()

    const events = within(screen.getByRole('region', { name: 'Life events' }).querySelector('ol')!).getAllByRole('listitem')
    expect(events.map((li) => li.querySelector('p')?.textContent)).toEqual([
      'Birth',
      'Census',
      'Marriage',
      'Deathdisputed',
    ])
    // GEDCOM dates are shown as plain English.
    expect(within(events[0]!).getByText('12 March 1890')).toBeInTheDocument()
    expect(screen.getByText('as child')).toBeInTheDocument()
    expect(screen.getByText('with Eva Weber')).toBeInTheDocument()
    expect(screen.getByText('Reason: Only family tradition')).toBeInTheDocument()

    const parents = screen.getByRole('region', { name: 'Parents and siblings' })
    expect(within(parents).getByRole('link', { name: 'Hans Weber' })).toBeInTheDocument()
    expect(within(parents).getByText('Unknown parent')).toBeInTheDocument()
    expect(within(parents).getByText('(adopted)')).toBeInTheDocument()
    expect(within(parents).getByRole('link', { name: 'Lena Weber' })).toBeInTheDocument()

    const partners = screen.getByRole('region', { name: 'Partners and children' })
    expect(within(partners).getByRole('link', { name: 'Eva Weber' })).toBeInTheDocument()
    expect(within(partners).getByRole('link', { name: 'Mia Weber' })).toBeInTheDocument()

    expect(await axeViolations(container)).toEqual([])
  })

  it('adds a child with the family choice and the shortcut c', async () => {
    const withTwo: PersonDetail = {
      ...paul,
      partnerFamilies: [...paul.partnerFamilies, family(22, { partner1: ref(1, 'Paul'), partner2: null })],
    }
    let sent: unknown
    mockApi({
      'GET /api/persons/1': [200, withTwo],
      'POST /api/persons/1/relatives': (body) => {
        sent = body
        return [201, { person: { ...paul, id: 9 }, family: withTwo.partnerFamilies[1] }]
      },
    })
    const user = userEvent.setup()
    renderApp('/people/1')
    await screen.findByRole('heading', { level: 1, name: 'Paul Weber' })

    await user.keyboard('c')
    const dialog = await screen.findByRole('dialog', { name: 'Add a child of Paul Weber' })
    // Children usually share the surname.
    expect(within(dialog).getByLabelText('Surname')).toHaveValue('Weber')
    await user.type(within(dialog).getByLabelText('Given names'), 'Tom')
    await user.selectOptions(within(dialog).getByLabelText('With which partner?'), 'other parent unknown')
    await user.selectOptions(within(dialog).getByLabelText('Relationship to the parent'), 'adopted')
    await user.click(within(dialog).getByRole('button', { name: /^Add/ }))

    await waitFor(() => expect(sent).toBeDefined())
    expect(sent).toMatchObject({
      relation: 'child',
      familyId: 22,
      childRelation: 'adopted',
      person: { givenNames: 'Tom', surname: 'Weber' },
    })
  })

  it('sends birth and death from the new-person form, death only when not living', async () => {
    const bodies: unknown[] = []
    mockApi({
      'GET /api/persons': [200, { items: [], total: 0 }],
      'GET /api/dates/parse': [200, { valid: true, empty: false, normalized: '12 MAR 1890', qualifier: '' }],
      'POST /api/persons': (body) => {
        bodies.push(body)
        return [201, { ...paul, id: 9 }]
      },
      'GET /api/persons/9': [200, { ...paul, id: 9 }],
    })
    const user = userEvent.setup()
    renderApp('/people')
    await screen.findByText(/No one is in your tree yet/)
    await user.keyboard('n')
    const dialog = await screen.findByRole('dialog', { name: 'New person' })

    const birth = within(within(dialog).getByRole('group', { name: 'Birth (optional)' }))
    const death = () => within(dialog).queryByRole('group', { name: 'Death (optional)' })
    expect(death()).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText('Given names'), 'Paul')
    await user.type(birth.getByLabelText('Date'), '12.3.1890')
    expect(await birth.findByText('Stored as 12 MAR 1890')).toBeInTheDocument()

    // Living people have no death fields.
    await user.selectOptions(within(dialog).getByLabelText('Living or deceased'), 'Living')
    expect(death()).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: /^Save/ }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toMatchObject({ givenNames: 'Paul', birth: { date: '12.3.1890', placeId: null } })
    expect(bodies[0]).not.toHaveProperty('death')
  })

  it('shows server field errors in the event dialog and previews dates', async () => {
    mockApi({
      'GET /api/persons/1': [200, paul],
      'GET /api/dates/parse': [200, { valid: true, empty: false, normalized: 'ABT 1850', qualifier: 'ABT' }],
      'POST /api/events': [422, { error: 'invalid input', fields: { customLabel: 'is required for custom events' } }],
    })
    const user = userEvent.setup()
    renderApp('/people/1')
    await screen.findByRole('heading', { level: 1, name: 'Paul Weber' })

    await user.keyboard('a')
    const dialog = await screen.findByRole('dialog', { name: 'New life event' })
    await user.type(within(dialog).getByLabelText('Date'), 'abt 1850')
    expect(await within(dialog).findByText('Understood as about 1850')).toBeInTheDocument()

    await user.selectOptions(within(dialog).getByLabelText('Event'), 'Other event')
    await user.click(within(dialog).getByRole('button', { name: /^Save/ }))
    const label = await within(dialog).findByLabelText('Name of the event')
    await waitFor(() => expect(label).toHaveAttribute('aria-invalid', 'true'))
    expect(label).toHaveAccessibleDescription('is required for custom events')
  })

  it('shows not found for a missing person', async () => {
    mockApi({})
    renderApp('/people/99')
    expect(await screen.findByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument()
  })
})

describe('discarding changes', () => {
  const openNewPerson = async () => {
    mockApi({
      'GET /api/persons': [200, { items: [], total: 0 }],
      'POST /api/persons': [201, paul],
      'GET /api/persons/1': [200, paul],
    })
    const user = userEvent.setup()
    renderApp('/people')
    await screen.findByText(/No one is in your tree yet/)
    await user.keyboard('n')
    const dialog = await screen.findByRole('dialog', { name: 'New person' })
    return { user, dialog }
  }
  // jsdom has no Escape-to-cancel, so fire the event the browser would.
  const pressEscape = (dialog: HTMLElement) => fireEvent(dialog, new Event('cancel', { cancelable: true }))
  const discardDialog = () => screen.queryByRole('dialog', { name: 'Discard changes?' })

  it('closes a clean form immediately', async () => {
    const { user, dialog } = await openNewPerson()
    pressEscape(dialog)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New person' })).not.toBeInTheDocument())
    expect(discardDialog()).not.toBeInTheDocument()

    await user.keyboard('n')
    await user.click(await screen.findByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog', { name: 'New person' })).not.toBeInTheDocument()
  })

  it('asks before an Escape closes a modified form, and keeps it on cancel', async () => {
    const { user, dialog } = await openNewPerson()
    await user.type(screen.getByLabelText('Given names'), 'Anna')
    pressEscape(dialog)

    const confirm = await screen.findByRole('dialog', { name: 'Discard changes?' })
    // The question replaces the form's content; the form keeps its input.
    expect(screen.getAllByRole('dialog')).toHaveLength(1)
    expect(screen.getByLabelText('Given names')).toHaveValue('Anna')
    // Escape on the question goes back to the form.
    pressEscape(confirm)
    await waitFor(() => expect(discardDialog()).not.toBeInTheDocument())
    expect(screen.getByRole('dialog', { name: 'New person' })).toBeInTheDocument()
    expect(screen.getByLabelText('Given names')).toHaveValue('Anna')
  })

  it('discards the input after confirming, from Cancel and from the close button', async () => {
    const { user } = await openNewPerson()
    await user.type(screen.getByLabelText('Given names'), 'Anna')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await user.click(within(await screen.findByRole('dialog', { name: 'Discard changes?' })).getByRole('button', { name: 'Discard' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New person' })).not.toBeInTheDocument())
    expect(discardDialog()).not.toBeInTheDocument()

    await user.keyboard('n')
    await user.type(await screen.findByLabelText('Given names'), 'Anna')
    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(await screen.findByRole('dialog', { name: 'Discard changes?' })).toBeInTheDocument()
  })

  it('does not ask after a successful save', async () => {
    const { user } = await openNewPerson()
    await user.type(screen.getByLabelText('Given names'), 'Anna')
    await user.click(screen.getByRole('button', { name: /^Save/ }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New person' })).not.toBeInTheDocument())
    expect(discardDialog()).not.toBeInTheDocument()
  })
})
