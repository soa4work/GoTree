import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Media } from '../api/types'
import { axeViolations } from '../test/axe'
import { mockApi, renderApp } from '../test/render'

const photo: Media = {
  id: 4,
  kind: 'image',
  mime: 'image/jpeg',
  title: 'Wedding 1950',
  width: 1200,
  height: 800,
  size: 1000,
  originalName: 'wedding.jpg',
  date: '1950',
  description: 'The couple in front of the church',
  transcript: '',
  takenAt: '1950-06-03 11:00:00',
  lat: 51.34,
  lng: 12.375,
  links: [{ entityType: 'person', entityId: 1, label: 'Anna Müller', personId: 1 }],
  regions: [
    {
      id: 9,
      person: { id: 1, givenNames: 'Anna', surname: 'Müller', sex: 'F', birthDate: '', deathDate: '', living: false, portrait: null },
      name: '',
      x: 0.1,
      y: 0.1,
      w: 0.2,
      h: 0.3,
      source: 'manual',
    },
    { id: 10, person: null, name: 'Grandma Rose', x: 0.5, y: 0.2, w: 0.2, h: 0.3, source: 'xmp' },
  ],
}

describe('media page', () => {
  it('shows a photo with its faces, camera data and links', async () => {
    mockApi({ 'GET /api/media/4': [200, photo] })
    const { container } = renderApp('/media/4')

    expect(await screen.findByRole('heading', { level: 1, name: 'Wedding 1950' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'The couple in front of the church' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Face: Anna Müller' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Face: Grandma Rose' })).toBeInTheDocument()
    expect(screen.getByText('Imported from the photo')).toBeInTheDocument()
    expect(screen.getByRole('searchbox', { name: 'Who is “Grandma Rose”?' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Location 51.34000, 12.37500/ })).toHaveAttribute('href', expect.stringContaining('openstreetmap.org'))
    expect(screen.getByRole('button', { name: 'Add as event for Anna Müller' })).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })

  it('offers face drawing as a toggle with a keyboard alternative explained', async () => {
    mockApi({ 'GET /api/media/4': [200, photo] })
    const user = userEvent.setup()
    renderApp('/media/4')
    const toggle = await screen.findByRole('button', { name: 'Mark a face' })
    await user.click(toggle)
    expect(screen.getByRole('button', { name: 'Drawing: drag over a face' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText(/Without a mouse or touch screen/)).toBeInTheDocument()
  })

  it('plays recordings with their transcript', async () => {
    mockApi({
      'GET /api/media/5': [
        200,
        { ...photo, id: 5, kind: 'audio', mime: 'audio/mpeg', title: 'Grandpa tells', regions: [], transcript: 'We came in 1923.' },
      ],
    })
    const { container } = renderApp('/media/5')
    await screen.findByRole('heading', { level: 1, name: 'Grandpa tells' })
    expect(container.querySelector('audio')).toHaveAttribute('src', '/api/media/5/file')
    expect(within(screen.getByRole('region', { name: 'Transcript' })).getByText('We came in 1923.')).toBeInTheDocument()
  })

  it('lists media on the person page with an upload button', async () => {
    mockApi({
      'GET /api/persons/1': [
        200,
        {
          id: 1, givenNames: 'Anna', surname: 'Müller', namePrefix: '', nameSuffix: '', nickname: '', sex: 'F',
          isLiving: null, living: false, notes: '', alternateNames: [], citations: [], portrait: { mediaId: 4, regionId: 9 },
          createdAt: '', updatedAt: '', events: [], parentFamilies: [], partnerFamilies: [],
        },
      ],
      'GET /api/media': [200, [{ id: 4, kind: 'image', mime: 'image/jpeg', title: 'Wedding 1950', width: 1200, height: 800 }]],
    })
    renderApp('/people/1')
    const section = await screen.findByRole('region', { name: 'Photos and documents' })
    expect(await within(section).findByRole('link', { name: 'Wedding 1950' })).toHaveAttribute('href', '/media/4')
    expect(within(section).getByLabelText('+ Upload files')).toHaveAttribute('type', 'file')
    // The portrait is the cropped face tag.
    const portrait = document.querySelector('img[src*="region=9"]')
    expect(portrait).not.toBeNull()
  })
})

describe('all media gallery', () => {
  it('loads the next page on demand', async () => {
    const tile = (id: number) => ({ id, kind: 'image', mime: 'image/jpeg', title: `Photo ${id}`, width: 10, height: 10 })
    const user = userEvent.setup()
    mockApi({
      'GET /api/media': (_body, url) =>
        url.searchParams.get('offset') === '120'
          ? [200, Array.from({ length: 5 }, (_, i) => tile(120 + i))]
          : [200, Array.from({ length: 120 }, (_, i) => tile(i))],
    })
    renderApp('/media')
    await screen.findByRole('link', { name: 'Photo 0' })
    expect(screen.getAllByRole('link', { name: /^Photo \d+$/ })).toHaveLength(120)
    await user.click(screen.getByRole('button', { name: 'Show more' }))
    await waitFor(() => expect(screen.getAllByRole('link', { name: /^Photo \d+$/ })).toHaveLength(125))
    expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument()
  })
})

describe('upload button', () => {
  it('uploads without an onUploaded callback', async () => {
    const fetchMock = mockApi({ 'POST /api/media': [201, { ...photo, id: 7 }] })
    const { QueryClient, QueryClientProvider } = await import('@tanstack/react-query')
    const { render, fireEvent, waitFor } = await import('@testing-library/react')
    const { UploadButton } = await import('./UploadButton')
    render(
      <QueryClientProvider client={new QueryClient()}>
        <UploadButton owner={{ entityType: 'person', entityId: 1 }} />
      </QueryClientProvider>,
    )
    fireEvent.change(screen.getByLabelText('+ Upload files'), {
      target: { files: [new File(['x'], 'a.jpg', { type: 'image/jpeg' })] },
    })
    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([url, init]) => String(url) === '/api/media?entityType=person&entityId=1' && init?.method === 'POST')).toBe(true),
    )
  })
})
