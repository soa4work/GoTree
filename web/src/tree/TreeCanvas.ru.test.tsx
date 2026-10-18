import { screen } from '@testing-library/react'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import type { PersonRef, TreeGraph } from '../api/types'
import i18n from '../i18n'
import { mockApi, renderApp } from '../test/render'

const person = (id: number, givenNames: string): PersonRef => ({
  id,
  givenNames,
  surname: 'Weber',
  sex: 'U',
  birthDate: '',
  deathDate: '',
  living: false,
  portrait: null,
})

const graph: TreeGraph = {
  rootId: 1,
  persons: { '1': person(1, 'Paul') },
  families: [],
  truncated: false,
}

describe('tree chart in Russian', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('ru')
  })
  afterAll(async () => {
    await i18n.changeLanguage('en')
  })

  it('translates the React Flow zoom controls', async () => {
    mockApi({ 'GET /api/tree/1': [200, graph] })
    const { container } = renderApp('/tree?root=1')
    expect(await screen.findByRole('button', { name: 'Увеличить' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Уменьшить' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Показать всю схему' })).toBeInTheDocument()
    expect(container.querySelector('[title="Zoom In"], [title="Fit View"], [aria-label="Zoom In"]')).toBeNull()
    expect(container.querySelector('[aria-describedby^="react-flow__node-desc"]')).toBeNull()
  })
})
