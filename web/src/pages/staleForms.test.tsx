import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Link, MemoryRouter, Route, Routes } from 'react-router'
import { describe, expect, it } from 'vitest'
import '../i18n'
import type { Media, RecordTemplate, Transcription } from '../api/types'
import { mockApi } from '../test/render'
import { MediaPage } from './MediaPage'
import { TranscriptionPage } from './TranscriptionPage'

const media = (id: number, title: string): Media => ({
  id,
  kind: 'image',
  mime: 'image/jpeg',
  title,
  width: 100,
  height: 100,
  size: 10,
  originalName: `${title}.jpg`,
  date: '',
  description: '',
  transcript: '',
  takenAt: '',
  lat: null,
  lng: null,
  links: [],
  regions: [],
})

const census = {
  key: 'census',
  name: 'Census',
  builtin: true,
  eventType: 'CENS',
  roles: [],
  columns: [],
} as unknown as RecordTemplate

const transcription = (updatedAt: string): Transcription => ({
  id: 5,
  templateKey: 'census',
  title: 'Leipzig 1890',
  source: null,
  page: '',
  date: '',
  place: null,
  notes: '',
  rows: [{ line: 'first', role: '', values: {}, action: 'new', personId: null }],
  status: 'draft',
  eventId: null,
  appliedAt: null,
  persons: {},
  createdAt: '2025-01-01T00:00:00Z',
  updatedAt,
})

describe('stale forms', () => {
  it('does not carry the media form over to the next medium', async () => {
    let sent: Record<string, unknown> | undefined
    mockApi({
      'GET /api/media/1': [200, media(1, 'First')],
      'GET /api/media/2': [200, media(2, 'Second')],
      'PUT /api/media/2': (body) => {
        sent = body as Record<string, unknown>
        return [200, media(2, 'Second')]
      },
    })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/media/1']}>
          <Routes>
            <Route
              path="media/:id"
              element={
                <>
                  <MediaPage />
                  <Link to="/media/2">next</Link>
                </>
              }
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    const field = await screen.findByLabelText('Title')
    expect(field).toHaveValue('First')
    fireEvent.change(field, { target: { value: 'Typed' } })
    fireEvent.click(screen.getByRole('link', { name: 'next' }))
    await waitFor(() => expect(screen.getByLabelText('Title')).toHaveValue('Second'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toBeDefined())
    expect(sent!.title).toBe('Second')
  })

  it('keeps unsaved transcription rows when the data is refetched', async () => {
    let updatedAt = '2025-01-01T00:00:00Z'
    mockApi({
      'GET /api/templates': [200, [census]],
      'GET /api/transcriptions/5': () => [200, transcription(updatedAt)],
    })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/transcribe/5']}>
          <Routes>
            <Route path="transcribe/:id" element={<TranscriptionPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    const line = await screen.findByLabelText('Line')
    await userEvent.setup().type(line, ' typed')
    expect(line).toHaveValue('first typed')

    updatedAt = '2025-02-02T00:00:00Z'
    await client.invalidateQueries({ queryKey: ['transcriptions', 5] })
    await waitFor(() => expect(client.getQueryData<Transcription>(['transcriptions', 5])?.updatedAt).toBe(updatedAt))
    expect(screen.getByLabelText('Line')).toHaveValue('first typed')
  })
})
