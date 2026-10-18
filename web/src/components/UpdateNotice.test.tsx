import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it } from 'vitest'
import '../i18n'
import { AuthGate } from '../auth/AuthGate'
import { markUpdateReady, resetUpdateReady } from '../lib/updateReady'
import { mockApi } from '../test/render'
import { UpdateNotice } from './UpdateNotice'

afterEach(() => resetUpdateReady())

describe('UpdateNotice', () => {
  it('offers the reload even if the update was ready before it mounted', () => {
    markUpdateReady()
    render(<UpdateNotice />)
    expect(screen.getByRole('button', { name: 'Reload now' })).toBeInTheDocument()
  })

  it('shows on the login page', async () => {
    mockApi({ 'GET /api/auth/state': [200, { setupRequired: false, user: null, tree: null }] })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <AuthGate>
            <p>app</p>
          </AuthGate>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    await screen.findByRole('button', { name: 'Log in' })
    expect(screen.queryByRole('button', { name: 'Reload now' })).not.toBeInTheDocument()
    act(() => markUpdateReady())
    expect(screen.getByRole('button', { name: 'Reload now' })).toBeInTheDocument()
  })
})
