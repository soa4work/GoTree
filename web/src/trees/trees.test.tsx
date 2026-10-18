import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { AuthState } from '../api/client'
import { App } from '../App'
import { createQueryClient } from '../queryClient'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'

const weber = { id: 1, name: 'Weber family', role: 'owner', current: true }
const schmidt = { id: 2, name: 'Schmidt family', role: 'editor', current: false }
const inSchmidt: AuthState = { ...loggedInState, tree: { id: 2, name: 'Schmidt family', role: 'editor' } }
const noTree: AuthState = { ...loggedInState, tree: null }

const home = {
  'GET /api/research/tasks': [200, []] as [number, unknown],
  'GET /api/onthisday': [200, { events: [] }] as [number, unknown],
}
const stats = (persons: number) => [200, { persons, families: 0, sources: 0 }] as [number, unknown]

const calls = (fetchMock: ReturnType<typeof mockApi>, key: string) =>
  fetchMock.mock.calls.filter(([url, init]) => `${(init as RequestInit | undefined)?.method ?? 'GET'} ${String(url)}` === key)

const bodyOf = (fetchMock: ReturnType<typeof mockApi>, key: string) => {
  const init = calls(fetchMock, key)[0]?.[1] as RequestInit | undefined
  return JSON.parse((init?.body as string | undefined) ?? 'null')
}

describe('tree switcher', () => {
  it('names the tree in the header and lists the trees with the current one marked', async () => {
    mockApi({ ...home, 'GET /api/stats': stats(7), 'GET /api/trees': [200, [weber, schmidt]] })
    const user = userEvent.setup()
    renderApp()
    await user.click(await screen.findByRole('button', { name: 'Tree: Weber family. Switch tree' }))
    const dialog = await screen.findByRole('dialog', { name: 'Your trees' })
    const current = await within(dialog).findByRole('button', { name: /Weber family/ })
    expect(current).toHaveAttribute('aria-current', 'true')
    expect(current).toHaveTextContent('current')
    expect(current).toHaveTextContent('Owner')
    expect(within(dialog).getByRole('button', { name: /Schmidt family/ })).not.toHaveAttribute('aria-current')
    expect(within(dialog).getByRole('link', { name: 'Manage members' })).toHaveAttribute('href', '/members')
    expect(await axeViolations(dialog)).toEqual([])
  })

  it('does not offer member management to a viewer or an editor', async () => {
    mockApi({ ...home, 'GET /api/auth/state': [200, inSchmidt], 'GET /api/stats': stats(7), 'GET /api/trees': [200, [{ ...weber, current: false }, { ...schmidt, current: true }]] })
    const user = userEvent.setup()
    renderApp()
    await user.click(await screen.findByRole('button', { name: /Schmidt family/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Your trees' })
    await within(dialog).findByRole('button', { name: /Weber family/ })
    expect(within(dialog).queryByRole('link', { name: 'Manage members' })).not.toBeInTheDocument()
  })

  it('switches to another tree and drops the data of the old one', async () => {
    let switched = false
    const fetchMock = mockApi({
      ...home,
      'GET /api/auth/state': () => [200, switched ? inSchmidt : loggedInState],
      'GET /api/stats': () => stats(switched ? 3 : 7),
      'GET /api/trees': () => [200, switched ? [{ ...weber, current: false }, { ...schmidt, current: true }] : [weber, schmidt]],
      'PUT /api/auth/tree': () => {
        switched = true
        return [200, inSchmidt]
      },
    })
    const user = userEvent.setup()
    renderApp()
    expect(await screen.findByText('7')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Weber family/ }))
    await user.click(await screen.findByRole('button', { name: /Schmidt family/ }))

    expect(await screen.findByRole('button', { name: 'Tree: Schmidt family. Switch tree' })).toBeInTheDocument()
    expect(await screen.findByText('3')).toBeInTheDocument()
    expect(screen.queryByText('7')).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(calls(fetchMock, 'PUT /api/auth/tree')).toHaveLength(1)
    expect(bodyOf(fetchMock, 'PUT /api/auth/tree')).toEqual({ treeId: 2 })
  })

  it('creates a tree and then switches to it', async () => {
    let switched = false
    const fetchMock = mockApi({
      ...home,
      'GET /api/auth/state': () => [200, switched ? { ...loggedInState, tree: { id: 3, name: 'Fischer family', role: 'owner' } } : loggedInState],
      'GET /api/stats': stats(7),
      'GET /api/trees': [200, [weber]],
      'POST /api/trees': [201, { id: 3, name: 'Fischer family', role: 'owner', current: false }],
      'PUT /api/auth/tree': () => {
        switched = true
        return [200, { ...loggedInState, tree: { id: 3, name: 'Fischer family', role: 'owner' } }]
      },
    })
    const user = userEvent.setup()
    renderApp()
    await user.click(await screen.findByRole('button', { name: /Weber family/ }))
    await user.type(await screen.findByLabelText('Name of the new tree'), 'Fischer family')
    await user.click(screen.getByRole('button', { name: 'Create tree' }))

    expect(await screen.findByRole('button', { name: 'Tree: Fischer family. Switch tree' })).toBeInTheDocument()
    expect(bodyOf(fetchMock, 'POST /api/trees')).toEqual({ name: 'Fischer family' })
    expect(bodyOf(fetchMock, 'PUT /api/auth/tree')).toEqual({ treeId: 3 })
  })

  it('shows the translated field error for an empty name', async () => {
    mockApi({
      ...home,
      'GET /api/stats': stats(7),
      'GET /api/trees': [200, [weber]],
      'POST /api/trees': [422, { error: 'invalid', fields: { name: 'is required' }, fieldCodes: { name: { code: 'required' } } }],
    })
    const user = userEvent.setup()
    renderApp()
    await user.click(await screen.findByRole('button', { name: /Weber family/ }))
    await user.click(await screen.findByRole('button', { name: 'Create tree' }))
    expect(await screen.findByText('is required')).toBeInTheDocument()
    expect(screen.getByLabelText('Name of the new tree')).toBeInvalid()
  })

  it('shows why a switch failed', async () => {
    mockApi({
      ...home,
      'GET /api/stats': stats(7),
      'GET /api/trees': [200, [weber, schmidt]],
      'PUT /api/auth/tree': [404, { error: 'not found', code: 'not_found' }],
    })
    const user = userEvent.setup()
    renderApp()
    await user.click(await screen.findByRole('button', { name: /Weber family/ }))
    await user.click(await screen.findByRole('button', { name: /Schmidt family/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/Could not switch/)
  })
})

describe('user without a tree', () => {
  it('shows the tree picker, switches and has no accessibility violations', async () => {
    let switched = false
    mockApi({
      ...home,
      'GET /api/auth/state': () => [200, switched ? inSchmidt : noTree],
      'GET /api/stats': stats(3),
      'GET /api/trees': [200, [{ ...weber, current: false }, { ...schmidt, current: false }]],
      'PUT /api/auth/tree': () => {
        switched = true
        return [200, inSchmidt]
      },
    })
    const user = userEvent.setup()
    const { container } = renderApp('/members')
    expect(await screen.findByRole('heading', { level: 1, name: 'Choose a tree' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /Weber family/ })).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])

    await user.click(screen.getByRole('button', { name: /Schmidt family/ }))
    expect(await screen.findByRole('button', { name: 'Tree: Schmidt family. Switch tree' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Choose a tree' })).not.toBeInTheDocument()
  })
})

describe('stale membership', () => {
  it('re-checks the auth state when a request answers 403 not_member', async () => {
    const fetchMock = mockApi({
      ...home,
      'GET /api/stats': [403, { error: 'not a member', code: 'not_member' }],
    })
    // The app's own client, which watches for these errors; renderApp's plain one does not.
    render(
      <QueryClientProvider client={createQueryClient()}>
        <MemoryRouter>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )
    await screen.findByRole('button', { name: /Weber family/ })
    await waitFor(() => expect(calls(fetchMock, 'GET /api/auth/state').length).toBeGreaterThanOrEqual(2))
  })
})
