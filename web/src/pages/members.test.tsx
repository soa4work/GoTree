import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'

const members = [
  { userId: 1, username: 'admin', displayName: '', role: 'owner' },
  { userId: 2, username: 'bob', displayName: 'Bob Weber', role: 'owner' },
  { userId: 3, username: 'carol', displayName: '', role: 'viewer' },
]
const asRole = (role: string) => ({ 'GET /api/auth/state': [200, { ...loggedInState, tree: { id: 1, name: 'Weber family', role } }] as [number, unknown] })
const bodyOf = (fetchMock: ReturnType<typeof mockApi>, key: string) => {
  const call = fetchMock.mock.calls.find(([url, init]) => `${(init as RequestInit | undefined)?.method ?? 'GET'} ${String(url)}` === key)
  return call ? JSON.parse(((call[1] as RequestInit).body as string | undefined) ?? 'null') : undefined
}
const called = (fetchMock: ReturnType<typeof mockApi>, key: string) =>
  fetchMock.mock.calls.some(([url, init]) => `${(init as RequestInit | undefined)?.method ?? 'GET'} ${String(url)}` === key)

describe('members page', () => {
  it('shows an owner the members, role selects and the add form', async () => {
    mockApi({ 'GET /api/trees/1/members': [200, members] })
    const { container } = renderApp('/members')
    expect(await screen.findByRole('heading', { level: 1, name: 'Members' })).toBeInTheDocument()
    expect(await screen.findByText('Bob Weber')).toBeInTheDocument()
    expect(screen.getByText('@carol')).toBeInTheDocument()
    expect(screen.getByLabelText('Role of Bob Weber')).toHaveValue('owner')
    expect(screen.getByLabelText('Role of carol')).toHaveValue('viewer')
    expect(screen.getByText('3 members')).toBeInTheDocument()
    expect(screen.getByLabelText('Name of the tree')).toHaveValue('Weber family')
    expect(screen.getByLabelText('Username of the account')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Viewer' })).toBeChecked()
    // The owner's own row has no Remove button; leaving is the button below.
    expect(screen.queryByRole('button', { name: /Remove: admin/ })).not.toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })

  it('adds a member as a viewer by default', async () => {
    const fetchMock = mockApi({
      'GET /api/trees/1/members': [200, members],
      'POST /api/trees/1/members': [201, { userId: 4, username: 'dave', displayName: 'Dave', role: 'viewer' }],
    })
    const user = userEvent.setup()
    renderApp('/members')
    await user.type(await screen.findByLabelText('Username of the account'), 'dave')
    await user.click(screen.getByRole('button', { name: 'Add member' }))
    expect(await screen.findByText('Dave was added.')).toBeInTheDocument()
    expect(bodyOf(fetchMock, 'POST /api/trees/1/members')).toEqual({ username: 'dave', role: 'viewer' })
    expect(screen.getByLabelText('Username of the account')).toHaveValue('')
  })

  it('shows the translated error for an account that cannot be added', async () => {
    mockApi({
      'GET /api/trees/1/members': [200, members],
      'POST /api/trees/1/members': [422, { error: 'invalid', fields: { username: 'cannot be added' }, fieldCodes: { username: { code: 'not_addable' } } }],
    })
    const user = userEvent.setup()
    renderApp('/members')
    await user.type(await screen.findByLabelText('Username of the account'), 'nobody')
    await user.click(screen.getByRole('button', { name: 'Add member' }))
    expect(await screen.findByText('cannot be added')).toBeInTheDocument()
    expect(screen.getByLabelText('Username of the account')).toBeInvalid()
  })

  it('changes a role', async () => {
    const fetchMock = mockApi({
      'GET /api/trees/1/members': [200, members],
      'PUT /api/trees/1/members/3': [200, { ...members[2], role: 'editor' }],
    })
    const user = userEvent.setup()
    renderApp('/members')
    await user.selectOptions(await screen.findByLabelText('Role of carol'), 'editor')
    await screen.findByLabelText('Role of carol')
    expect(bodyOf(fetchMock, 'PUT /api/trees/1/members/3')).toEqual({ role: 'editor' })
  })

  it('shows the error of the last owner and puts the role back', async () => {
    mockApi({
      'GET /api/trees/1/members': [200, [members[0], { ...members[1], role: 'viewer' }]],
      'PUT /api/trees/1/members/1': [409, { error: 'a tree needs an owner', code: 'last_owner' }],
    })
    const user = userEvent.setup()
    renderApp('/members')
    const select = await screen.findByLabelText('Role of admin')
    await user.selectOptions(select, 'editor')
    expect(await screen.findByRole('alert')).toHaveTextContent('A tree needs at least one owner. Make someone else an owner first.')
    expect(screen.getByLabelText('Role of admin')).toHaveValue('owner')
  })

  it('asks before removing a member', async () => {
    const fetchMock = mockApi({
      'GET /api/trees/1/members': [200, members],
      'DELETE /api/trees/1/members/2': [204],
    })
    const user = userEvent.setup()
    renderApp('/members')
    await user.click(await screen.findByRole('button', { name: /Remove: Bob Weber/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Remove Bob Weber?' })
    expect(called(fetchMock, 'DELETE /api/trees/1/members/2')).toBe(false)
    await user.click(within(dialog).getByRole('button', { name: 'Remove from tree' }))
    await screen.findByText('Bob Weber')
    expect(called(fetchMock, 'DELETE /api/trees/1/members/2')).toBe(true)
    // The row and its button are gone, so the focus moves to the list.
    await waitFor(() => expect(screen.getByRole('region', { name: 'Members' })).toHaveFocus())
  })

  it('renames the tree', async () => {
    const fetchMock = mockApi({
      'GET /api/trees/1/members': [200, members],
      'PUT /api/trees/1': [200, { id: 1, name: 'Weber-Schmidt', role: 'owner', current: true }],
    })
    const user = userEvent.setup()
    renderApp('/members')
    const field = await screen.findByLabelText('Name of the tree')
    await user.clear(field)
    await user.type(field, 'Weber-Schmidt')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByLabelText('Name of the tree')
    expect(bodyOf(fetchMock, 'PUT /api/trees/1')).toEqual({ name: 'Weber-Schmidt' })
  })

  it('shows a viewer no member list, only the role and Leave; leaving shows the tree picker', async () => {
    let left = false
    const fetchMock = mockApi({
      ...asRole('viewer'),
      'GET /api/auth/state': () => [200, left ? { ...loggedInState, tree: null } : { ...loggedInState, tree: { id: 1, name: 'Weber family', role: 'viewer' } }],
      'GET /api/trees': [200, []],
      'DELETE /api/trees/1/members/1': () => {
        left = true
        return [204]
      },
    })
    const user = userEvent.setup()
    const { container } = renderApp('/members')
    expect(await screen.findByText('Only the owners of this tree manage its members.')).toBeInTheDocument()
    expect(screen.getByText('Your role in this tree: Viewer.')).toBeInTheDocument()
    expect(screen.queryByLabelText('Username of the account')).not.toBeInTheDocument()
    expect(called(fetchMock, 'GET /api/trees/1/members')).toBe(false)
    expect(await axeViolations(container)).toEqual([])

    await user.click(screen.getByRole('button', { name: 'Leave this tree' }))
    const dialog = await screen.findByRole('dialog', { name: 'Leave this tree?' })
    await user.click(within(dialog).getByRole('button', { name: 'Leave this tree' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Choose a tree' })).toBeInTheDocument()
    expect(called(fetchMock, 'DELETE /api/trees/1/members/1')).toBe(true)
  })

  it('shows the last-owner error inside the leave dialog', async () => {
    mockApi({
      'GET /api/trees/1/members': [200, members],
      'DELETE /api/trees/1/members/1': [409, { error: 'a tree needs an owner', code: 'last_owner' }],
    })
    const user = userEvent.setup()
    renderApp('/members')
    await user.click(await screen.findByRole('button', { name: 'Leave this tree' }))
    const dialog = await screen.findByRole('dialog', { name: 'Leave this tree?' })
    await user.click(within(dialog).getByRole('button', { name: 'Leave this tree' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('A tree needs at least one owner')
  })
})

describe('members counter in Russian', () => {
  it('picks the plural form', () => {
    const t = i18n.getFixedT('ru')
    expect(t('members.count', { count: 1 })).toBe('1 участник')
    expect(t('members.count', { count: 2 })).toBe('2 участника')
    expect(t('members.count', { count: 5 })).toBe('5 участников')
    expect(t('members.count', { count: 22 })).toBe('22 участника')
  })
})
