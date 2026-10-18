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
