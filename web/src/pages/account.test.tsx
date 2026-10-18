import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { SessionInfo } from '../api/client'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'

const sessions: SessionInfo[] = [
  {
    id: 1,
    createdAt: '2026-10-01T10:00:00Z',
    lastSeenAt: '2026-10-05T10:00:00Z',
    expiresAt: '2026-11-04T10:00:00Z',
    userAgent: 'Firefox on Linux',
    current: true,
  },
  {
    id: 2,
    createdAt: '2026-09-20T10:00:00Z',
    lastSeenAt: '2026-10-02T10:00:00Z',
    expiresAt: '2026-11-01T10:00:00Z',
    userAgent: 'Safari on iPhone',
    current: false,
  },
]

const listing = { 'GET /api/auth/sessions': [200, sessions] as [number, unknown] }

const callsTo = (fetchMock: ReturnType<typeof mockApi>, method: string, path: string) =>
  fetchMock.mock.calls.filter(([url, init]) => String(url).endsWith(path) && (init?.method ?? 'GET') === method)

async function fillPasswordForm(user: ReturnType<typeof userEvent.setup>, current: string, next: string, repeat: string) {
  await user.type(await screen.findByLabelText('Current password'), current)
  await user.type(screen.getByLabelText('New password'), next)
  await user.type(screen.getByLabelText('Repeat password'), repeat)
  await user.click(screen.getByRole('button', { name: 'Change password' }))
}

describe('account page', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists the signed-in devices', async () => {
    mockApi(listing)
    const { container } = renderApp('/account')
    expect(await screen.findByRole('heading', { level: 1, name: 'Account' })).toBeInTheDocument()
    expect(await screen.findByText('Safari on iPhone')).toBeInTheDocument()
    expect(screen.getAllByText('This device')).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: /^Sign out(?!.*everywhere)/ })).toHaveLength(1)
    expect(await axeViolations(container)).toEqual([])
  })

  it('signs another device out', async () => {
    const fetchMock = mockApi({ ...listing, 'DELETE /api/auth/sessions/2': [204] })
    const user = userEvent.setup()
    renderApp('/account')
    await user.click(await screen.findByRole('button', { name: /Sign out: Safari on iPhone/ }))
    await vi.waitFor(() => expect(callsTo(fetchMock, 'DELETE', '/api/auth/sessions/2')).toHaveLength(1))
  })

  it('checks that the new passwords match before sending anything', async () => {
    const fetchMock = mockApi(listing)
    const user = userEvent.setup()
    renderApp('/account')
    await fillPasswordForm(user, 'old password here', 'a brand new password', 'something else')
    expect(screen.getByLabelText('Repeat password')).toHaveAccessibleDescription('The passwords do not match.')
    expect(callsTo(fetchMock, 'POST', '/api/auth/password')).toHaveLength(0)
  })

  it('shows a wrong current password next to its field', async () => {
    mockApi({
      ...listing,
      'POST /api/auth/password': [
        422,
        {
          error: 'invalid input',
          code: 'invalid_input',
          fields: { currentPassword: 'is not your current password' },
          fieldCodes: { currentPassword: { code: 'wrong_password' } },
        },
      ],
    })
    const user = userEvent.setup()
    renderApp('/account')
    await fillPasswordForm(user, 'not my password', 'a brand new password', 'a brand new password')
    const field = await screen.findByLabelText('Current password')
    await vi.waitFor(() => expect(field).toHaveAccessibleDescription('is not your current password'))
  })

  it('confirms a password change', async () => {
    const fetchMock = mockApi({ ...listing, 'POST /api/auth/password': [204] })
    const user = userEvent.setup()
    renderApp('/account')
    await fillPasswordForm(user, 'old password here', 'a brand new password', 'a brand new password')
    expect(await screen.findByText(/Your password was changed/)).toBeInTheDocument()
    const body = JSON.parse(String(callsTo(fetchMock, 'POST', '/api/auth/password')[0]?.[1]?.body))
    expect(body).toEqual({ currentPassword: 'old password here', newPassword: 'a brand new password' })
  })

  it('signs out everywhere and lands on the login page', async () => {
    let signedOut = false
    const fetchMock = mockApi({
      ...listing,
      'GET /api/auth/state': () => [200, signedOut ? { setupRequired: false, user: null, tree: null } : loggedInState],
      'POST /api/auth/logout-all': () => {
        signedOut = true
        return [204]
      },
    })
    const user = userEvent.setup()
    renderApp('/account')
    await user.click(await screen.findByRole('button', { name: 'Sign out everywhere' }))
    const dialog = await screen.findByRole('dialog', { name: 'Sign out everywhere?' })
    await user.click(within(dialog).getByRole('button', { name: 'Sign out everywhere' }))
    expect(await screen.findByRole('heading', { name: 'Log in' })).toBeInTheDocument()
    expect(callsTo(fetchMock, 'POST', '/api/auth/logout-all')).toHaveLength(1)
  })
})
