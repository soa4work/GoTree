import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { ImportReport } from '../api/types'
import { ImportReportView } from './ImportReportView'

const report: ImportReport = {
  version: '7.0',
  encoding: 'UTF-8',
  source: 'GoTree',
  counts: { persons: 3, media: 2 },
  tags: [],
  warnings: [],
  invalidDates: 0,
  invalidDateExamples: [],
  brokenReferences: [],
  childrenFromFamc: 0,
  childrenFromFamcExamples: [],
  shareLinksKept: 0,
  shareLinksLost: [],
  durationMs: 100,
}

describe('ImportReportView', () => {
  it('shows how many photos and documents were imported', () => {
    render(
      <MemoryRouter>
        <ImportReportView report={report} />
      </MemoryRouter>,
    )
    const label = screen.getByText('Photos and documents')
    expect(label.parentElement).toHaveTextContent('2')
  })
})
