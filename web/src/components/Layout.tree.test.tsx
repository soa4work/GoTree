import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { loggedInState, mockApi, renderApp } from '../test/render'

afterEach(() => i18n.changeLanguage('en'))

const withTree = (name: string) => ({ ...loggedInState, tree: { ...loggedInState.tree!, name } })

describe('tree name in the header', () => {
  it('shows the default name in Russian', async () => {
    await i18n.changeLanguage('ru')
    mockApi({ 'GET /api/auth/state': [200, withTree('Family tree')] })
    renderApp('/no-such-page')
    expect(await screen.findByText('Семейное дерево')).toBeInTheDocument()
  })

  it('shows a chosen name unchanged', async () => {
    await i18n.changeLanguage('ru')
    mockApi({ 'GET /api/auth/state': [200, withTree('Weber family')] })
    renderApp('/no-such-page')
    expect(await screen.findByText('Weber family')).toBeInTheDocument()
  })
})
