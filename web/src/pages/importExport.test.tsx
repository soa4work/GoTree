import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { loggedInState, mockApi, renderApp } from '../test/render'

const people = (total: number) => ({ 'GET /api/persons': [200, { items: [], total }] as [number, unknown] })
const asEditor = { 'GET /api/auth/state': [200, { ...loggedInState, tree: { id: 1, name: 'Weber family', role: 'editor' } }] as [number, unknown] }

const branchLink = {
  id: 1,
  label: 'Weber branch',
  scope: 'descendants',
  root: { id: 5, givenNames: 'Anna', surname: 'Weber', sex: 'F', birthDate: '', deathDate: '', living: false, portrait: null },
  privacy: 'deceased',
  expiresAt: null,
  revokedAt: null,
  lastUsedAt: null,
  createdAt: '2026-01-01T00:00:00Z',
}
const shareLinks = (links: unknown[]) => ({ 'GET /api/share-links': [200, links] as [number, unknown] })

describe('import page', () => {
  it('warns that branch share links may be deleted when replacing', async () => {
    mockApi({
      ...people(3),
      ...shareLinks([
        branchLink,
        { ...branchLink, id: 2, revokedAt: '2026-02-01T00:00:00Z' },
        { ...branchLink, id: 3, expiresAt: '2020-01-01T00:00:00Z' },
        { ...branchLink, id: 4, scope: 'tree', root: null },
      ]),
    })
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    expect(await screen.findByText(/1 share link shows one branch/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Replace the tree' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/1 share link shows one branch/)).toBeInTheDocument()
  })

  it('does not mention share links when there are none', async () => {
    mockApi({ ...people(3), ...shareLinks([]) })
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    expect(await screen.findByRole('button', { name: 'Replace the tree' })).toBeEnabled()
    expect(screen.queryByText(/share link/)).not.toBeInTheDocument()
  })

  it('offers the owner to replace a tree that has data', async () => {
    mockApi(people(3))
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    expect(await screen.findByRole('button', { name: 'Replace the tree' })).toBeEnabled()
    expect(screen.queryByText(/Only the owner of the tree/)).not.toBeInTheDocument()
  })

  it('offers an owner the export with everyone', async () => {
    mockApi(people(3))
    renderApp('/import-export')
    expect(await screen.findByRole('radio', { name: 'Include everyone' })).toBeChecked()
    expect(screen.queryByText(/export the tree without living people/)).not.toBeInTheDocument()
  })

  it('offers a viewer no export with living people', async () => {
    mockApi({ ...people(3), 'GET /api/auth/state': [200, { ...loggedInState, tree: { id: 1, name: 'Weber family', role: 'viewer' } }] })
    renderApp('/import-export')
    expect(await screen.findByText(/export the tree without living people/)).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: 'Include everyone' })).not.toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Leave living people out' })).toBeChecked()
    expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute('href', expect.stringContaining('privacy=exclude-living'))
  })

  it('disables replacing for an editor and says why', async () => {
    mockApi({ ...people(3), ...asEditor })
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    expect(await screen.findByText(/Only the owner of the tree can replace it/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Replace the tree' })).toBeDisabled()
  })

  describe('report of children added from their own FAMC', () => {
    const report = (childrenFromFamc: number) => ({
      version: '5.5.1',
      encoding: 'UTF-8',
      source: 'Test',
      counts: { persons: 5, families: 1 },
      tags: [],
      warnings: [],
      invalidDates: 0,
      invalidDateExamples: [],
      brokenReferences: [],
      childrenFromFamc,
      shareLinksKept: 0,
      shareLinksLost: [] as string[],
      childrenFromFamcExamples:
        childrenFromFamc > 0
          ? ['line 20: @I4@ added as a child of family @F1@', 'line 24: @I5@ added as a child of family @F1@']
          : [],
      durationMs: 10,
    })
    const runImport = async (childrenFromFamc: number) => {
      mockApi({ ...people(0), 'POST /api/import/gedcom': [200, report(childrenFromFamc)] as [number, unknown] })
      renderApp('/import-export')
      await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
      await userEvent.click(await screen.findByRole('button', { name: 'Import' }))
    }

    it('lists children added from their own FAMC in the report', async () => {
      await runImport(2)
      expect(await screen.findByText('2 children added to families by their INDI.FAMC lines')).toBeInTheDocument()
      expect(screen.getByText('line 20: @I4@ added as a child of family @F1@')).toBeInTheDocument()
    })

    it('says nothing when no child was added that way', async () => {
      await runImport(0)
      await screen.findByText('Test', { exact: false })
      expect(screen.queryByText(/INDI\.FAMC lines/)).not.toBeInTheDocument()
    })
  })

  it('lists share links deleted by the replacement', async () => {
    mockApi({
      ...people(0),
      'POST /api/import/gedcom': [
        200,
        {
          version: '5.5.1',
          encoding: 'UTF-8',
          source: 'Test',
          counts: {},
          tags: [],
          warnings: [],
          invalidDates: 0,
          invalidDateExamples: [],
          brokenReferences: [],
          childrenFromFamc: 0,
          childrenFromFamcExamples: [],
          shareLinksKept: 1,
          shareLinksLost: ['Roth branch'],
          durationMs: 10,
        },
      ] as [number, unknown],
    })
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('GEDCOM file'), new File(['0 HEAD'], 'tree.ged'))
    await userEvent.click(await screen.findByRole('button', { name: 'Import' }))
    expect(await screen.findByText('1 share link deleted')).toBeInTheDocument()
    expect(screen.getByText('Roth branch')).toBeInTheDocument()
    expect(screen.getByText(/1 share link to a branch kept/)).toBeInTheDocument()
  })

  it('lets an editor import into an empty tree', async () => {
    mockApi({ ...people(0), ...asEditor })
    renderApp('/import-export')
    expect(await screen.findByRole('button', { name: 'Import' })).toBeInTheDocument()
    expect(screen.queryByText(/Only the owner of the tree/)).not.toBeInTheDocument()
  })

  describe('export check', () => {
    const checkReport = { ok: true, version: '5.5.1', rows: [{ kind: 'Persons', tree: 4, roundTrip: 4 }], warnings: [], dropped: [] }

    it('runs the check as a POST', async () => {
      mockApi({ ...people(0), 'POST /api/export/verify': [200, checkReport] as [number, unknown] })
      renderApp('/import-export')
      await userEvent.click(await screen.findByRole('button', { name: 'Check the export' }))
      expect(await screen.findByText('Persons')).toBeInTheDocument()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })

    it('hides the check from viewers', async () => {
      mockApi({
        ...people(0),
        'GET /api/auth/state': [200, { ...loggedInState, tree: { id: 1, name: 'Weber family', role: 'viewer' } }] as [number, unknown],
      })
      renderApp('/import-export')
      await screen.findByLabelText('GEDCOM file')
      expect(screen.queryByRole('button', { name: 'Check the export' })).not.toBeInTheDocument()
    })

    it('shows the translated message when another check is running', async () => {
      mockApi({
        ...people(0),
        'POST /api/export/verify': [429, { code: 'export_check_busy', error: 'busy' }] as [number, unknown],
      })
      renderApp('/import-export')
      await userEvent.click(await screen.findByRole('button', { name: 'Check the export' }))
      expect(await screen.findByText(/An export check is already running/)).toBeInTheDocument()
    })
  })
})

describe('backup section', () => {
  it('tells administrators that the backup holds every tree and account', async () => {
    mockApi(people(0))
    renderApp('/import-export')
    expect(await screen.findByText(/The backup holds every tree and every account on this server/)).toBeInTheDocument()
  })

  it('leaves the backup section out for users who are not administrators', async () => {
    mockApi({
      ...people(0),
      'GET /api/auth/state': [200, { ...loggedInState, user: { id: 2, username: 'bob', displayName: '', role: 'user' } }] as [number, unknown],
    })
    renderApp('/import-export')
    await screen.findByLabelText('GEDCOM file')
    expect(screen.queryByText(/The backup holds every tree/)).not.toBeInTheDocument()
  })
})
