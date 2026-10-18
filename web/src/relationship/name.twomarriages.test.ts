import { afterAll, describe, expect, it } from 'vitest'
import type { PersonRef, RelationshipReport, Sex } from '../api/types'
import i18n from '../i18n'
import { relationshipName } from './name'
import { relationshipPath } from './path'

const t = i18n.t.bind(i18n)
const previous = i18n.language

afterAll(async () => {
  await i18n.changeLanguage(previous)
})

const person = (id: number, sex: Sex): PersonRef => ({
  id,
  givenNames: `P${id}`,
  surname: '',
  sex,
  birthDate: '',
  deathDate: '',
  living: false,
  portrait: null,
})

const kin = (up: number, down: number, path: number[], half = false) => ({
  up,
  down,
  half,
  adoptive: false,
  ancestors: [],
  path,
})

/** A is 1 and B is 2; via is 3 and via2 is 4; 10 is the shared parent. */
function spouseBloodSpouse(bSex: Sex, viaSex: Sex, via2Sex: Sex, half = false): RelationshipReport {
  return {
    a: 1,
    b: 2,
    kind: 'spouse_blood_spouse',
    kinship: kin(1, 1, [1, 3, 10, 4, 2], half),
    via: 3,
    via2: 4,
    others: [],
    persons: { 1: person(1, 'U'), 2: person(2, bSex), 3: person(3, viaSex), 4: person(4, via2Sex), 10: person(10, 'M') },
  }
}

/** Step-sibling (A, parent 3, step-parent 4, B) or co-parent-in-law (A, child 3, child's spouse 4, B). */
function fourPeople(kind: 'step_sibling' | 'co_parent_in_law', bSex: Sex): RelationshipReport {
  return {
    a: 1,
    b: 2,
    kind,
    kinship: kin(kind === 'step_sibling' ? 1 : 0, kind === 'step_sibling' ? 0 : 1, [1, 3, 4, 2]),
    via: 3,
    via2: 4,
    others: [],
    persons: { 1: person(1, 'U'), 2: person(2, bSex), 3: person(3, 'F'), 4: person(4, 'M') },
  }
}

const cases: [string, RelationshipReport, Record<string, string>][] = [
  ['husbands of sisters', spouseBloodSpouse('M', 'F', 'F'), { en: 'co-brother-in-law', ru: 'свояк', kk: 'бажа' }],
  ['wives of brothers', spouseBloodSpouse('F', 'M', 'M'), { en: 'co-sister-in-law', ru: 'жена брата мужа', kk: 'абысын' }],
  ['wife’s brother’s wife', spouseBloodSpouse('F', 'F', 'M'), { en: 'brother-in-law’s wife', ru: 'жена брата жены', kk: 'әйелінің бауырының әйелі' }],
  ['husband’s sister’s husband', spouseBloodSpouse('M', 'M', 'F'), { en: 'sister-in-law’s husband', ru: 'муж сестры мужа', kk: 'күйеуінің қыз бауырының күйеуі' }],
  ['stepbrother', fourPeople('step_sibling', 'M'), { en: 'stepbrother', ru: 'сводный брат', kk: 'өгей бауыр' }],
  ['stepsister', fourPeople('step_sibling', 'F'), { en: 'stepsister', ru: 'сводная сестра', kk: 'өгей бауыр' }],
  ['stepsibling of unknown sex', fourPeople('step_sibling', 'U'), { en: 'stepsibling', ru: 'сводная сестра или сводный брат', kk: 'өгей бауыр' }],
  ['child’s spouse’s father', fourPeople('co_parent_in_law', 'M'), { en: 'co-parent-in-law', ru: 'сват', kk: 'құда' }],
  ['child’s spouse’s mother', fourPeople('co_parent_in_law', 'F'), { en: 'co-parent-in-law', ru: 'сватья', kk: 'құдағи' }],
]

describe.each(['en', 'ru', 'kk'])('names through two marriages: %s', (lang) => {
  it.each(cases)('%s', async (_, rep, want) => {
    await i18n.changeLanguage(lang)
    expect(relationshipName(t, rep)).toBe(want[lang])
  })
})

describe('path through two marriages', () => {
  it('lists the people with how each is linked', () => {
    expect(relationshipPath(spouseBloodSpouse('M', 'F', 'F'))).toEqual([
      { id: 1, link: null },
      { id: 3, link: 'spouse' },
      { id: 10, link: 'parent' },
      { id: 4, link: 'child' },
      { id: 2, link: 'spouse' },
    ])
    expect(relationshipPath(fourPeople('step_sibling', 'M')).map((s) => s.link)).toEqual([null, 'parent', 'spouse', 'child'])
    expect(relationshipPath(fourPeople('co_parent_in_law', 'M')).map((s) => s.link)).toEqual([null, 'child', 'spouse', 'parent'])
  })
})
