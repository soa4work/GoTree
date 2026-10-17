import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MapData, PersonRef, UnmappedPlace } from '../api/types'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'
import { positionsAt, trailsUntil, yearRange } from './positions'

// jsdom has no layout for Leaflet; the table carries the same content.
vi.mock('./MapView', () => ({ default: () => <div data-testid="map" /> }))

const person = (id: number, givenNames: string): PersonRef => ({
  id,
  givenNames,
  surname: 'Weber',
  sex: 'U',
  birthDate: '',
  deathDate: '',
  living: false,
  portrait: null,
})

const data: MapData = {
  places: {
    1: { name: 'Kleindorf, Sachsen', lat: 51, lng: 13, approximate: true },
    2: { name: 'Leipzig, Sachsen', lat: 51.34, lng: 12.37, approximate: false },
  },
  persons: { 7: person(7, 'Hans'), 8: person(8, 'Paul') },
  tracks: [
    { personId: 7, points: [{ key: 18500000, placeId: 1, type: 'BIRT' }, { key: 18800000, placeId: 2, type: 'RESI' }], death: 19200000 },
    { personId: 8, points: [{ key: 18850301, placeId: 2, type: 'BIRT' }], death: 0 },
  ],
  unmapped: 3,
}

describe('map positions', () => {
  it('moves people with their events and ends at death', () => {
    expect(yearRange(data, 2026)).toEqual([1850, 1920])
    expect([...positionsAt(data, 1850)]).toEqual([[1, [7]]])
    expect([...positionsAt(data, 1879)]).toEqual([[1, [7]]])
    expect([...positionsAt(data, 1885)]).toEqual([[2, [7, 8]]])
    expect([...positionsAt(data, 1921)]).toEqual([[2, [8]]])
    // Without a death, a person leaves the map a century after their first event.
    expect(positionsAt(data, 1986).size).toBe(0)
  })

  it('draws the moves made so far by the people on the map', () => {
    expect(trailsUntil(data, 1879)).toEqual([])
    expect(trailsUntil(data, 1890)).toEqual([{ from: 1, to: 2, count: 1, last: 1880 }])
    // Hans died in 1920, so his move is no longer drawn.
    expect(trailsUntil(data, 1921)).toEqual([])
  })
})

describe('map page', () => {
  it('lists who was where in the chosen year', async () => {
    mockApi({ 'GET /api/map': [200, data] })
    const { container } = renderApp('/map')
    const slider = await screen.findByRole('slider', { name: /Year/ })
    expect(screen.getByText('1 person on the map in 1850.')).toBeInTheDocument()
    fireEvent.change(slider, { target: { value: '1890' } })
    const table = screen.getByRole('table')
    const rows = within(table).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('Leipzig, SachsenHans WeberPaul Weber')
    expect(screen.getByRole('heading', { name: 'Who was where in 1890' })).toBeInTheDocument()
    expect(screen.getByText(/3 events are at places without coordinates/)).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })
})

const atlantis: UnmappedPlace = {
  id: 5,
  parentId: 2,
  name: 'Atlantis',
  placeType: 'city',
  lat: null,
  lng: null,
  notes: 'sunk',
  fullName: 'Atlantis, Sachsen',
  events: 2,
}
const empty: MapData = { places: {}, persons: {}, tracks: [], unmapped: 2 }

describe('map without coordinates', () => {
  it('explains the empty map and links to the places to fix', async () => {
    mockApi({
      'GET /api/map': [200, empty],
      'GET /api/map/unmapped-places': [200, { total: 1, places: [atlantis] }],
    })
    const { container } = renderApp('/map')
    expect(await screen.findByText('No dated events at places with coordinates yet.')).toBeInTheDocument()
    expect(screen.getByText(/does not look them up by itself/)).toBeInTheDocument()
    expect(await screen.findByText(/1 place used by dated events has no coordinates/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add coordinates' })).toHaveAttribute('href', '#map-missing-places')
    expect(screen.getByRole('heading', { name: 'Places without coordinates' })).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })

  it('validates coordinates and saves them on the place', async () => {
    const fetchMock = mockApi({
      'GET /api/map': [200, empty],
      'GET /api/map/unmapped-places': [200, { total: 1, places: [atlantis] }],
      'PUT /api/places/5': [200, { ...atlantis, lat: 36.5, lng: -5.2 }],
    })
    renderApp('/map')
    const save = await screen.findByRole('button', {
      name: /Save coordinates/,
    })
    fireEvent.change(screen.getByLabelText('Latitude'), {
      target: { value: '36,5' },
    })
    fireEvent.click(save)
    expect(await screen.findByText('Enter both latitude and longitude as numbers.')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Longitude'), {
      target: { value: '200' },
    })
    fireEvent.click(save)
    expect(await screen.findByText('Longitude must be between -180 and 180.')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Longitude'), {
      target: { value: '-5.2' },
    })
    fireEvent.click(save)
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(([url, init]) => String(url) === '/api/places/5' && init?.method === 'PUT')
      expect(JSON.parse(String(put?.[1]?.body))).toEqual({
        parentId: 2,
        name: 'Atlantis',
        placeType: 'city',
        notes: 'sunk',
        lat: 36.5,
        lng: -5.2,
      })
    })
  })

  it('looks a place up on OpenStreetMap only on click and fills in the picked match', async () => {
    const api = mockApi({
      'GET /api/map': [200, empty],
      'GET /api/map/unmapped-places': [200, { total: 1, places: [atlantis] }],
    })
    const nominatim = vi.fn<(url: string, init?: RequestInit) => Promise<Response>>(() =>
      Promise.resolve(
        new Response(
          JSON.stringify([
            {
              display_name: 'Atlantis, Cadiz, Spain',
              lat: '36.5',
              lon: '-6.2',
            },
          ]),
          {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          },
        ),
      ),
    )
    vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) =>
      String(input).startsWith('https://nominatim.openstreetmap.org/') ? nominatim(String(input), init) : api(input, init),
    )
    const { container } = renderApp('/map')
    const lookup = await screen.findByRole('button', {
      name: /Look up on OpenStreetMap/,
    })
    expect(nominatim).not.toHaveBeenCalled()
    fireEvent.click(lookup)
    fireEvent.click(await screen.findByRole('button', { name: /Use these coordinates/ }))
    expect(nominatim).toHaveBeenCalledTimes(1)
    expect(nominatim.mock.calls[0]?.[0]).toContain('q=Atlantis%2C+Sachsen')
    expect(nominatim.mock.calls[0]?.[1]?.referrerPolicy).toBe('strict-origin-when-cross-origin')
    expect(screen.getByLabelText('Latitude')).toHaveValue('36.5')
    expect(screen.getByLabelText('Longitude')).toHaveValue('-6.2')
    expect(await axeViolations(container)).toEqual([])
  })

  it('lists the places read-only for viewers and says how many more there are', async () => {
    mockApi({
      'GET /api/auth/state': [
        200,
        {
          ...loggedInState,
          tree: { id: 1, name: 'Weber family', role: 'viewer' },
        },
      ],
      'GET /api/map': [200, empty],
      'GET /api/map/unmapped-places': [200, { total: 250, places: [atlantis] }],
    })
    renderApp('/map')
    expect(await screen.findByText('Atlantis, Sachsen')).toBeInTheDocument()
    expect(screen.getByText(/250 places used by dated events have no coordinates/)).toBeInTheDocument()
    expect(screen.getByText('250 places lack coordinates; showing the 1 busiest.')).toBeInTheDocument()
    expect(screen.getByText('Only editors can add coordinates.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Save coordinates/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Look up on OpenStreetMap/ })).not.toBeInTheDocument()
  })
})
