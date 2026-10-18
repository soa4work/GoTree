import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { vi } from 'vitest'
import type { AuthState } from '../api/client'
import { App } from '../App'

export function renderApp(path = '/') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

type Reply = [status: number, body?: unknown]
type Handler = Reply | ((body: unknown, url: URL) => Reply)

export const loggedInState: AuthState = {
  setupRequired: false,
  user: { id: 1, username: 'admin', displayName: '', role: 'admin' },
  tree: { id: 1, name: 'Weber family', role: 'owner' },
}

export const healthy = { status: 'ok', version: '1.2.3', database: 'ok' }

/**
 * Stubs fetch with a tiny fake API keyed by "METHOD /api/path". Unlisted
 * routes answer 404. Returns the mock so tests can inspect the calls.
 */
export function mockApi(routes: Record<string, Handler>) {
  const all: Record<string, Handler> = {
    'GET /api/auth/state': [200, loggedInState],
    'GET /api/health': [200, healthy],
    ...routes,
  }
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost')
    const key = `${init?.method ?? 'GET'} ${url.pathname}`
    const handler = all[key]
    const body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
    const [status, payload] = handler === undefined ? [404, { error: 'not found' }] : typeof handler === 'function' ? handler(body, url) : handler
    return Promise.resolve(
      status === 204
        ? new Response(null, { status })
        : new Response(JSON.stringify(payload ?? {}), { status, headers: { 'Content-Type': 'application/json' } }),
    )
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
