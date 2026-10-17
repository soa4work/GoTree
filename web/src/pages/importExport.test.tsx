import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { loggedInState, mockApi, renderApp } from '../test/render'

const people = (total: number) => ({ 'GET /api/persons': [200, { items: [], total }] as [number, unknown] })
const asEditor = { 'GET /api/auth/state': [200, { ...loggedInState, tree: { id: 1, name: 'Weber family', role: 'editor' } }] as [number, unknown] }

describe('import page', () => {
  it('offers the owner to replace a tree that has data', async () => {
    mockApi(people(3))
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    expect(await screen.findByRole('button', { name: 'Replace the tree' })).toBeEnabled()
    expect(screen.queryByText(/Only the owner of the tree/)).not.toBeInTheDocument()
  })

  it('disables replacing for an editor and says why', async () => {
    mockApi({ ...people(3), ...asEditor })
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    expect(await screen.findByText(/Only the owner of the tree can replace it/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Replace the tree' })).toBeDisabled()
  })

  it('lets an editor import into an empty tree', async () => {
    mockApi({ ...people(0), ...asEditor })
    renderApp('/import-export')
    expect(await screen.findByRole('button', { name: 'Import' })).toBeInTheDocument()
    expect(screen.queryByText(/Only the owner of the tree/)).not.toBeInTheDocument()
  })
})
