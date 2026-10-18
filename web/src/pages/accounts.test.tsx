import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'

const accounts = [
  {
    id: 1,
    username: 'admin',
    displayName: '',
    role: 'admin',
    createdAt: '2026-01-01T10:00:00Z',
    lastSeenAt: '2026-03-05T10:00:00Z',
    trees: [{ id: 1, name: 'Weber family', role: 'owner' }],
  },
  {
    id: 2,
    username: 'bob',
    displayName: 'Bob Weber',
    role: 'user',
    createdAt: '2026-02-01T10:00:00Z',
    lastSeenAt: '',
    trees: [],
  },
]

const bodyOf = (fetchMock: ReturnType<typeof mockApi>, key: string) => {
  const call = fetchMock.mock.calls.find(([url, init]) => `${(init as RequestInit | undefined)?.method ?? 'GET'} ${String(url)}` === key)
  return call ? JSON.parse(((call[1] as RequestInit).body as string | undefined) ?? 'null') : undefined
}
const called = (fetchMock: ReturnType<typeof mockApi>, key: string) =>
  fetchMock.mock.calls.some(([url, init]) => `${(init as RequestInit | undefined)?.method ?? 'GET'} ${String(url)}` === key)

describe('accounts page', () => {
  it('lists the accounts with their trees and badges', async () => {
    mockApi({ 'GET /api/admin/users': [200, accounts] })
    const { container } = renderApp('/admin/users')
    expect(await screen.findByRole('heading', { level: 1, name: 'Accounts' })).toBeInTheDocument()
    expect(await screen.findByText('Bob Weber')).toBeInTheDocument()
    expect(screen.getByText('@admin')).toBeInTheDocument()
    expect(screen.getByText('@bob')).toBeInTheDocument()
    expect(screen.getByText('Administrator', { selector: 'span' })).toBeInTheDocument()
    expect(screen.getByText('Weber family (Owner)')).toBeInTheDocument()
    expect(screen.getByText('Not in any tree yet')).toBeInTheDocument()
    expect(screen.getByText('Has not signed in yet')).toBeInTheDocument()
    expect(screen.getByText(/^Last active .*2026/)).toBeInTheDocument()
    expect(screen.getByText('2 accounts')).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })

  it('creates an account and points to the members page', async () => {
    const fetchMock = mockApi({
      'GET /api/admin/users': [200, accounts],
      'POST /api/admin/users': [201, { ...accounts[1], id: 3, username: 'carol', displayName: 'Carol' }],
    })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: '+ New account' }))
    const dialog = await screen.findByRole('dialog', { name: 'New account' })
    await user.type(within(dialog).getByLabelText('Username'), 'carol')
    await user.type(within(dialog).getByLabelText('Display name'), 'Carol')
    await user.type(within(dialog).getByLabelText(/^New password/), 'a long enough password')
    await user.type(within(dialog).getByLabelText('Repeat password'), 'a long enough password')
    expect(within(dialog).getByRole('radio', { name: /^User/ })).toBeChecked()
    await user.click(within(dialog).getByRole('button', { name: /^Create account/ }))
    expect(await screen.findByText(/The account Carol was created\./)).toBeInTheDocument()
    expect(bodyOf(fetchMock, 'POST /api/admin/users')).toEqual({
      username: 'carol',
      displayName: 'Carol',
      password: 'a long enough password',
      role: 'user',
    })
    expect(screen.getByRole('link', { name: 'Members page' })).toHaveAttribute('href', '/members')
  })

  it('sends nothing when the passwords differ', async () => {
    const fetchMock = mockApi({ 'GET /api/admin/users': [200, accounts], 'POST /api/admin/users': [201, accounts[1]] })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: '+ New account' }))
    const dialog = await screen.findByRole('dialog', { name: 'New account' })
    await user.type(within(dialog).getByLabelText('Username'), 'carol')
    await user.type(within(dialog).getByLabelText(/^New password/), 'a long enough password')
    await user.type(within(dialog).getByLabelText('Repeat password'), 'something else entirely')
    await user.click(within(dialog).getByRole('button', { name: /^Create account/ }))
    expect(await within(dialog).findByText('The passwords do not match.')).toBeInTheDocument()
    expect(called(fetchMock, 'POST /api/admin/users')).toBe(false)
  })

  it('shows the translated error of a username that is taken', async () => {
    mockApi({
      'GET /api/admin/users': [200, accounts],
      'POST /api/admin/users': [
        422,
        { error: 'invalid', fields: { username: 'is already taken' }, fieldCodes: { username: { code: 'taken' } } },
      ],
    })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: '+ New account' }))
    const dialog = await screen.findByRole('dialog', { name: 'New account' })
    await user.type(within(dialog).getByLabelText('Username'), 'bob')
    await user.type(within(dialog).getByLabelText(/^New password/), 'a long enough password')
    await user.type(within(dialog).getByLabelText('Repeat password'), 'a long enough password')
    await user.click(within(dialog).getByRole('button', { name: /^Create account/ }))
    expect(await within(dialog).findByText('is already taken')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Username')).toBeInvalid()
  })

  it('edits the name and the role', async () => {
    const fetchMock = mockApi({
      'GET /api/admin/users': [200, accounts],
      'PUT /api/admin/users/2': [200, { ...accounts[1], role: 'admin' }],
    })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: /^Edit: bob/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Edit Bob Weber' })
    const name = within(dialog).getByLabelText('Display name')
    expect(name).toHaveValue('Bob Weber')
    await user.clear(name)
    await user.type(name, 'Robert Weber')
    await user.click(within(dialog).getByRole('radio', { name: /^Administrator/ }))
    await user.click(within(dialog).getByRole('button', { name: /^Save/ }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(bodyOf(fetchMock, 'PUT /api/admin/users/2')).toEqual({ displayName: 'Robert Weber', role: 'admin' })
  })

  it('resets a password, and has no reset or delete on the own row', async () => {
    const fetchMock = mockApi({
      'GET /api/admin/users': [200, accounts],
      'POST /api/admin/users/2/password': [204],
    })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: /^Reset password: bob/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Reset the password of Bob Weber' })
    await user.type(within(dialog).getByLabelText(/^New password/), 'another long password')
    await user.type(within(dialog).getByLabelText('Repeat password'), 'another long password')
    await user.click(within(dialog).getByRole('button', { name: /^Reset password/ }))
    expect(await screen.findByText('The password of Bob Weber was reset.')).toBeInTheDocument()
    expect(bodyOf(fetchMock, 'POST /api/admin/users/2/password')).toEqual({ password: 'another long password' })
    // The own row (admin) can be edited only.
    expect(screen.getByRole('button', { name: /^Edit: admin/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Reset password: admin/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Delete: admin/ })).not.toBeInTheDocument()
  })

  it('asks before deleting an account', async () => {
    const fetchMock = mockApi({
      'GET /api/admin/users': [200, accounts],
      'DELETE /api/admin/users/2': [204],
    })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: /^Delete: bob/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Delete the account Bob Weber?' })
    expect(called(fetchMock, 'DELETE /api/admin/users/2')).toBe(false)
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(called(fetchMock, 'DELETE /api/admin/users/2')).toBe(true))
    await waitFor(() => expect(screen.getByRole('region', { name: 'Accounts' })).toHaveFocus())
  })

  it('shows the translated message when the account is the last owner of a tree', async () => {
    mockApi({
      'GET /api/admin/users': [200, accounts],
      'DELETE /api/admin/users/2': [409, { error: 'a tree needs at least one owner', code: 'last_owner' }],
    })
    const user = userEvent.setup()
    renderApp('/admin/users')
    await user.click(await screen.findByRole('button', { name: /^Delete: bob/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Delete the account Bob Weber?' })
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('A tree needs at least one owner. Make someone else an owner first.')
  })

  it('tells a user who is not an administrator that the page is not for them', async () => {
    mockApi({
      'GET /api/auth/state': [200, { ...loggedInState, user: { id: 2, username: 'bob', displayName: '', role: 'user' } }],
      'GET /api/admin/users': [403, { error: 'only administrators can do this', code: 'admin_only' }],
    })
    renderApp('/admin/users')
    expect(await screen.findByText('Only administrators manage accounts.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '+ New account' })).not.toBeInTheDocument()
  })

  it('lets an administrator without a tree reach the page', async () => {
    mockApi({
      'GET /api/auth/state': [200, { ...loggedInState, tree: null }],
      'GET /api/trees': [200, []],
      'GET /api/admin/users': [200, accounts],
    })
    renderApp('/')
    const link = await screen.findByRole('link', { name: 'Manage accounts' })
    expect(link).toHaveAttribute('href', '/admin/users')
    await userEvent.setup().click(link)
    expect(await screen.findByText('Bob Weber')).toBeInTheDocument()
  })

  it('does not offer the link to a user who is not an administrator without a tree', async () => {
    mockApi({
      'GET /api/auth/state': [200, { ...loggedInState, tree: null, user: { id: 2, username: 'bob', displayName: '', role: 'user' } }],
      'GET /api/trees': [200, []],
    })
    renderApp('/admin/users')
    expect(await screen.findByRole('heading', { name: 'Choose a tree' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Manage accounts' })).not.toBeInTheDocument()
  })
})

describe('accounts page in Russian', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('ru')
  })
  afterAll(async () => {
    await i18n.changeLanguage('en')
  })

  it('picks the Russian plural form of the account count', () => {
    const t = i18n.getFixedT('ru')
    expect(t('accounts.count', { count: 1 })).toBe('1 учётная запись')
    expect(t('accounts.count', { count: 2 })).toBe('2 учётные записи')
    expect(t('accounts.count', { count: 5 })).toBe('5 учётных записей')
    expect(t('accounts.count', { count: 21 })).toBe('21 учётная запись')
    expect(t('accounts.count', { count: 22 })).toBe('22 учётные записи')
  })
})
