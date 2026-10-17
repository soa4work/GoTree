import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { axeViolations } from '../test/axe'
import { loggedInState, mockApi, renderApp } from '../test/render'

const stats = (persons: number, families: number) => ({ persons, families })

describe('home page', () => {
  it('offers to add yourself or import when the tree is empty', async () => {
    mockApi({ 'GET /api/stats': [200, stats(0, 0)], 'GET /api/sources': [200, []], 'GET /api/research/tasks': [200, []] })
    const { container } = renderApp('/')
    expect(await screen.findByRole('heading', { level: 2, name: 'No one is in your tree yet' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add yourself' })).toHaveAttribute('href', '/people?new=1')
    expect(screen.getByRole('link', { name: 'Import from Ancestry, MyHeritage and others' })).toHaveAttribute(
      'href',
      '/import-export',
    )
    expect(screen.queryByRole('heading', { name: 'Your tree' })).not.toBeInTheDocument()
    expect(await axeViolations(container)).toEqual([])
  })

  it('summarises a filled tree and hides an empty "On this day"', async () => {
    mockApi({
      'GET /api/stats': [200, stats(12, 4)],
      'GET /api/sources': [200, [{ id: 1 }, { id: 2 }, { id: 3 }]],
      'GET /api/research/tasks': [200, []],
      'GET /api/onthisday': [200, { events: [] }],
    })
    const { container } = renderApp('/')
    expect(await screen.findByRole('heading', { level: 2, name: 'Your tree' })).toBeInTheDocument()
    expect(await screen.findByText('12')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(await screen.findByText('3')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Add yourself' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /On this day/ })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Wall chart' })).toHaveAttribute('href', '/chart')
    expect(await axeViolations(container)).toEqual([])
  })

  it('shows the server version to administrators only', async () => {
    mockApi({
      'GET /api/auth/state': [200, { ...loggedInState, user: { ...loggedInState.user!, role: 'user' } }],
      'GET /api/stats': [200, stats(1, 0)],
    })
    renderApp('/')
    await screen.findByRole('heading', { level: 2, name: 'Your tree' })
    expect(screen.queryByText(/Server online/)).not.toBeInTheDocument()
  })
})
