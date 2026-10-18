import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { LoginPage } from './LoginPage'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'

const loggedOut = { setupRequired: false, user: null, tree: null }

describe('first-run setup', () => {
  it('creates the admin and lands in the app', async () => {
    const fetchMock = mockApi({
      'GET /api/auth/state': [200, { setupRequired: true, user: null, tree: null }],
      'POST /api/auth/setup': [200, loggedInState],
    })
    const user = userEvent.setup()
    const { container } = renderApp()

    expect(await screen.findByRole('heading', { level: 1, name: 'Set up GoTree' })).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])

    await user.type(screen.getByLabelText('Name of your family tree'), 'Weber family')
    await user.type(screen.getByLabelText(/^Password/), 'correct horse battery')
    await user.type(screen.getByLabelText('Repeat password'), 'correct horse battery')
    await user.click(screen.getByRole('button', { name: 'Create account and start' }))

    expect(await screen.findByRole('navigation', { name: 'Main navigation' })).toBeInTheDocument()
    const setupCall = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(String(setupCall?.[1]?.body))).toEqual({
      username: 'admin',
      password: 'correct horse battery',
      treeName: 'Weber family',
    })
  })

  it.each([
    ['en', 'Family tree'],
    ['ru', 'Семейное дерево'],
  ])('names an unnamed tree in %s', async (lang, name) => {
    await i18n.changeLanguage(lang)
    const fetchMock = mockApi({
      'GET /api/auth/state': [200, { setupRequired: true, user: null, tree: null }],
      'POST /api/auth/setup': [200, loggedInState],
    })
    const user = userEvent.setup()
    renderApp()
    await screen.findByRole('heading', { level: 1 })
    const fields = screen.getAllByLabelText(/^(Password|Repeat password|Пароль|Повторите пароль)/)
    for (const f of fields) await user.type(f, 'correct horse battery')
    await user.click(screen.getByRole('button', { name: /^(Create account and start|Создать)/ }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(true))
    const call = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(String(call?.[1]?.body)).treeName).toBe(name)
    await i18n.changeLanguage('en')
  })

  it('checks that the passwords match before sending anything', async () => {
    const fetchMock = mockApi({ 'GET /api/auth/state': [200, { setupRequired: true, user: null, tree: null }] })
    const user = userEvent.setup()
    renderApp()
    await screen.findByRole('heading', { name: 'Set up GoTree' })

    await user.type(screen.getByLabelText(/^Password/), 'correct horse battery')
    await user.type(screen.getByLabelText('Repeat password'), 'something else')
    await user.click(screen.getByRole('button', { name: 'Create account and start' }))

    const confirm = screen.getByLabelText('Repeat password')
    expect(confirm).toHaveAttribute('aria-invalid', 'true')
    expect(confirm).toHaveAccessibleDescription('The passwords do not match.')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  })

  it('shows server-side field errors next to the field', async () => {
    mockApi({
      'GET /api/auth/state': [200, { setupRequired: true, user: null, tree: null }],
      'POST /api/auth/setup': [422, { error: 'invalid input', fields: { password: 'must be at least 10 characters' } }],
    })
    const user = userEvent.setup()
    renderApp()
    await screen.findByRole('heading', { name: 'Set up GoTree' })
    await user.type(screen.getByLabelText(/^Password/), 'short')
    await user.type(screen.getByLabelText('Repeat password'), 'short')
    await user.click(screen.getByRole('button', { name: 'Create account and start' }))

    await waitFor(() => expect(screen.getByLabelText(/^Password/)).toHaveAttribute('aria-invalid', 'true'))
    expect(screen.getByLabelText(/^Password/)).toHaveAccessibleDescription(
      expect.stringContaining('must be at least 10 characters'),
    )
  })
})

describe('login', () => {
  it('logs in and out', async () => {
    let state: unknown = loggedOut
    mockApi({
      'GET /api/auth/state': () => [200, state],
      'POST /api/auth/login': (body) => {
        const { password } = body as { password: string }
        if (password !== 'correct horse battery') return [401, { error: 'wrong username or password' }]
        state = loggedInState
        return [204]
      },
      'POST /api/auth/logout': () => {
        state = loggedOut
        return [204]
      },
    })
    const user = userEvent.setup()
    const { container } = renderApp()

    expect(await screen.findByRole('heading', { level: 1, name: 'Log in' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Renée French' })).toHaveAttribute('href', 'https://reneefrench.blogspot.com/')
    expect(screen.getByRole('link', { name: 'CC BY 4.0' })).toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])

    await user.type(screen.getByLabelText('Username'), 'admin')
    await user.type(screen.getByLabelText('Password'), 'wrong')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Wrong username or password.')

    await user.clear(screen.getByLabelText('Password'))
    await user.type(screen.getByLabelText('Password'), 'correct horse battery')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('button', { name: /Weber family/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Log out' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Log in' })).toBeInTheDocument()
  })

  it('explains throttling', async () => {
    mockApi({
      'GET /api/auth/state': [200, loggedOut],
      'POST /api/auth/login': [429, { error: 'too many failed logins' }],
    })
    const user = userEvent.setup()
    renderApp()
    await screen.findByRole('heading', { name: 'Log in' })
    await user.type(screen.getByLabelText('Username'), 'admin')
    await user.type(screen.getByLabelText('Password'), 'whatever')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('wait 15 minutes')
  })
})

describe('login cache', () => {
  it('drops data cached for a previous user after logging in', async () => {
    mockApi({ 'POST /api/auth/login': [204], 'GET /api/auth/state': [200, loggedInState] })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    client.setQueryData(['person', 1], { id: 1, givenNames: 'Previous' })
    client.setQueryData(['auth'], loggedOut)
    render(
      <QueryClientProvider client={client}>
        <LoginPage />
      </QueryClientProvider>,
    )
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Username'), 'admin')
    await user.type(screen.getByLabelText('Password'), 'pw')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    await waitFor(() => expect(client.getQueryData(['person', 1])).toBeUndefined())
    expect(client.getQueryData(['auth'])).toBeDefined()
  })
})
