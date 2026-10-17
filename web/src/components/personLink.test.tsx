import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { mockApi, renderApp } from '../test/render'

describe('person page', () => {
  it('links to the tree centred on the person', async () => {
    mockApi({
      'GET /api/persons/4': [
        200,
        {
          id: 4,
          givenNames: 'Anna',
          surname: 'Weber',
          namePrefix: '',
          nameSuffix: '',
          nickname: '',
          sex: 'F',
          isLiving: null,
          living: false,
          notes: '',
          alternateNames: [],
          citations: [],
          portrait: null,
          createdAt: '',
          updatedAt: '',
          events: [],
          parentFamilies: [],
          partnerFamilies: [],
        },
      ],
      'GET /api/research/suggestions': [200, { suggestions: [], persons: {} }],
      'GET /api/research/tasks': [200, []],
      'GET /api/research/log': [200, []],
      'GET /api/checks': [200, { findings: [], persons: {}, taskIds: {} }],
      'GET /api/media': [200, []],
    })
    renderApp('/people/4')
    expect(await screen.findByRole('link', { name: 'View in tree' })).toHaveAttribute('href', '/tree?root=4')
  })
})
