import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { PersonDetail } from '../api/types'
import i18n from '../i18n'
import { mockApi, renderApp } from '../test/render'

afterEach(() => i18n.changeLanguage('en'))

const person = (sex: 'M' | 'F' | 'U'): PersonDetail => ({
  id: 1,
  givenNames: 'Ann',
  surname: 'Weber',
  namePrefix: '',
  nameSuffix: '',
  nickname: '',
  nameNotes: '',
  sex,
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
})

describe('person line in Russian', () => {
  it.each([
    ['F', /Женский · умерла/],
    ['M', /Мужской · умер$/],
    ['U', /умер\(ла\)$/],
  ] as const)('agrees the verb with sex %s', async (sex, want) => {
    await i18n.changeLanguage('ru')
    mockApi({ 'GET /api/persons/1': [200, person(sex)] })
    renderApp('/people/1')
    expect(await screen.findByText(want)).toBeInTheDocument()
  })
})

describe('shared person line in Russian', () => {
  it('agrees the verb with the sex', async () => {
    await i18n.changeLanguage('ru')
    mockApi({
      'GET /api/auth/state': [200, { setupRequired: false, user: null, tree: null }],
      'GET /api/share/tok/persons/1': [200, person('F')],
      'GET /api/share/tok': [
        200,
        { treeName: 'Family tree', label: '', scope: 'tree', privacy: 'living_names', root: null, startId: 1 },
      ],
    })
    renderApp('/share/tok/people/1')
    expect(await screen.findByText(/Женский · умерла/)).toBeInTheDocument()
    expect(screen.getAllByText(/Семейное дерево/).length).toBeGreaterThan(0)
  })
})
