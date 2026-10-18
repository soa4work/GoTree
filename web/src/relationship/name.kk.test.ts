import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import type { Kinship, PersonRef, RelationshipReport, Sex } from '../api/types'
import i18n from '../i18n'
import { kinshipName, relationshipName } from './name'

const t = i18n.t.bind(i18n)
let previous = i18n.language

beforeAll(async () => {
  previous = i18n.language
  await i18n.changeLanguage('kk')
})
afterAll(async () => {
  await i18n.changeLanguage(previous)
})

/** A person; `born` is the birth sort key (0 = unknown). */
const p = (id: number, sex: Sex, born = 0): PersonRef => ({
  id,
  givenNames: `P${id}`,
  surname: '',
  sex,
  birthDate: '',
  ...(born ? { birthSort: born } : {}),
  deathDate: '',
  living: false,
  portrait: null,
})

const kin = (up: number, down: number, path: number[], extra: Partial<Kinship> = {}): Kinship => ({
  up,
  down,
  half: false,
  adoptive: false,
  ancestors: [],
  path,
  ...extra,
})

const byId = (people: PersonRef[]) => Object.fromEntries(people.map((x) => [x.id, x]))

/** The blood term for B, the last of the people along the path from the ego. */
function blood(up: number, down: number, people: PersonRef[], extra: Partial<Kinship> = {}): string {
  const last = people[people.length - 1]
  return kinshipName(t, kin(up, down, people.map((x) => x.id), extra), last?.sex, byId(people))
}

const OLD = 19500101
const MID = 19800101
const YOUNG = 20100101

describe('kk: siblings by seniority and ego sex', () => {
  it.each<[string, Sex, number, Sex, number, string]>([
    ['elder brother of a man', 'M', MID, 'M', OLD, 'аға'],
    ['elder brother of a woman', 'F', MID, 'M', OLD, 'аға'],
    ['elder sister of a man', 'M', MID, 'F', OLD, 'әпке'],
    ['elder sister of a woman', 'F', MID, 'F', OLD, 'әпке'],
    ['younger brother of a man', 'M', MID, 'M', YOUNG, 'іні'],
    ['younger brother of a woman', 'F', MID, 'M', YOUNG, 'іні'],
    ['younger sister of a man', 'M', MID, 'F', YOUNG, 'қарындас'],
    ['younger sister of a woman', 'F', MID, 'F', YOUNG, 'сіңлі'],
    ['younger sister of an ego of unknown sex', 'U', MID, 'F', YOUNG, 'қарындас немесе сіңлі'],
    ['elder sibling of unknown sex', 'M', MID, 'U', OLD, 'әпке немесе аға'],
    ['younger sibling of unknown sex, ego man', 'M', MID, 'U', YOUNG, 'қарындас немесе іні'],
    ['unknown age, brother', 'M', 0, 'M', OLD, 'бауыр'],
    ['unknown age, sister', 'M', MID, 'F', 0, 'қыз бауыр'],
    ['unknown age, sibling of unknown sex', 'F', 0, 'U', 0, 'бауыр'],
    ['same birth day', 'M', MID, 'M', MID, 'бауыр'],
  ])('%s', (_, egoSex, egoBorn, sex, born, want) => {
    expect(blood(1, 1, [p(1, egoSex, egoBorn), p(10, 'M'), p(2, sex, born)])).toBe(want)
  })

  it('marks half-siblings by the parent they lack', () => {
    const people = [p(1, 'M', MID), p(10, 'M'), p(2, 'M', OLD), p(20, 'M'), p(21, 'F')]
    const half = (ancestor: number) => kinshipName(t, kin(1, 1, [1, 10, 2], { half: true, ancestors: [ancestor] }), 'M', byId(people))
    expect(half(20)).toBe('аға (әкесі бір)')
    expect(half(21)).toBe('аға (шешесі бір)')
    expect(half(-3)).toBe('аға (жартылай)')
  })
})

describe('kk: parents and ancestors', () => {
  const father = p(10, 'M')
  const mother = p(11, 'F')
  it.each<[string, number, PersonRef[], string]>([
    ['father', 1, [p(1, 'M'), p(2, 'M')], 'әке'],
    ['mother', 1, [p(1, 'M'), p(2, 'F')], 'шеше'],
    ['parent of unknown sex', 1, [p(1, 'M'), p(2, 'U')], 'ата-ана'],
    ['paternal grandfather', 2, [p(1, 'M'), father, p(2, 'M')], 'ата'],
    ['paternal grandmother', 2, [p(1, 'M'), father, p(2, 'F')], 'әже'],
    ['maternal grandfather', 2, [p(1, 'M'), mother, p(2, 'M')], 'нағашы ата'],
    ['maternal grandmother', 2, [p(1, 'F'), mother, p(2, 'F')], 'нағашы әже'],
    ['grandparent of unknown sex', 2, [p(1, 'M'), father, p(2, 'U')], 'әже немесе ата'],
    ['maternal grandparent of unknown sex', 2, [p(1, 'M'), mother, p(2, 'U')], 'нағашы әже немесе нағашы ата'],
    ['grandmother through an unknown parent', 2, [p(1, 'M'), p(-1, 'U'), p(2, 'F')], 'әже'],
    ['great-grandfather', 3, [p(1, 'M'), father, p(12, 'M'), p(2, 'M')], 'арғы ата'],
    ['great-grandmother', 3, [p(1, 'M'), mother, p(12, 'M'), p(2, 'F')], 'арғы әже'],
    ['4th generation', 4, [p(1, 'M'), father, p(12, 'M'), p(13, 'M'), p(2, 'M')], '4-ші буын ата-бабасы'],
    ['12th generation', 12, [p(1, 'M'), p(2, 'F')], '12-ші буын ата-бабасы'],
  ])('%s', (_, up, people, want) => {
    expect(blood(up, 0, people)).toBe(want)
  })

  it('names a grandparent without a path by the plain term', () => {
    expect(kinshipName(t, { up: 2, down: 0, half: false, adoptive: false }, 'F')).toBe('әже')
    expect(kinshipName(t, { up: 3, down: 0, half: false, adoptive: false }, 'M')).toBe('арғы ата')
  })
})

describe('kk: descendants', () => {
  it.each<[string, number, PersonRef[], string]>([
    ['son', 1, [p(1, 'M'), p(2, 'M')], 'ұл'],
    ['daughter', 1, [p(1, 'M'), p(2, 'F')], 'қыз'],
    ['child of unknown sex', 1, [p(1, 'M'), p(2, 'U')], 'бала'],
    ['son’s child', 2, [p(1, 'M'), p(10, 'M'), p(2, 'F')], 'немере'],
    ['daughter’s child', 2, [p(1, 'M'), p(10, 'F'), p(2, 'M')], 'жиен'],
    ['child of an unknown child', 2, [p(1, 'M'), p(10, 'U'), p(2, 'M')], 'немере'],
    ['great-grandchild', 3, [p(1, 'M'), p(10, 'F'), p(11, 'M'), p(2, 'M')], 'шөбере'],
    ['great-great-grandchild', 4, [p(1, 'M'), p(2, 'M')], 'шөпшек'],
    ['next generation', 5, [p(1, 'M'), p(2, 'F')], 'немене'],
    ['6th generation', 6, [p(1, 'M'), p(2, 'F')], '6-шы буын ұрпағы'],
  ])('%s', (_, down, people, want) => {
    expect(blood(0, down, people)).toBe(want)
  })
})

describe('kk: uncles, aunts, nephews and cousins', () => {
  const father = p(10, 'M', MID)
  const mother = p(11, 'F', MID)
  const gp = p(12, 'M')
  it.each<[string, PersonRef[], string]>([
    ['father’s elder brother', [p(1, 'M', YOUNG), father, gp, p(2, 'M', OLD)], 'көке'],
    ['father’s younger brother', [p(1, 'M', YOUNG), father, gp, p(2, 'M', YOUNG)], 'әкесінің інісі'],
    ['father’s brother, age unknown', [p(1, 'M', YOUNG), father, gp, p(2, 'M')], 'әкесінің бауыры'],
    ['father’s elder sister', [p(1, 'F', YOUNG), father, gp, p(2, 'F', OLD)], 'тәте'],
    ['father’s younger sister', [p(1, 'F', YOUNG), father, gp, p(2, 'F', YOUNG)], 'тәте'],
    ['father’s sibling of unknown sex', [p(1, 'M', YOUNG), father, gp, p(2, 'U', OLD)], 'тәте немесе көке'],
    ['mother’s brother', [p(1, 'M', YOUNG), mother, gp, p(2, 'M', OLD)], 'нағашы аға'],
    ['mother’s younger brother', [p(1, 'M', YOUNG), mother, gp, p(2, 'M', YOUNG)], 'нағашы аға'],
    ['mother’s sister', [p(1, 'F', YOUNG), mother, gp, p(2, 'F', OLD)], 'нағашы апа'],
    ['unknown parent’s elder brother', [p(1, 'M', YOUNG), p(10, 'U', MID), gp, p(2, 'M', OLD)], 'ата-анасының ағасы'],
    ['unknown parent’s younger sister', [p(1, 'M', YOUNG), p(10, 'U', MID), gp, p(2, 'F', YOUNG)], 'ата-анасының қарындасы немесе сіңлісі'],
    ['unknown parent’s sibling, age unknown', [p(1, 'M'), p(10, 'U'), gp, p(2, 'F')], 'ата-анасының бауыры'],
  ])('%s', (_, people, want) => {
    expect(blood(2, 1, people)).toBe(want)
  })

  it('names a grandparent’s sibling like a grandparent', () => {
    const path = [p(1, 'M'), father, gp, p(13, 'M')]
    expect(blood(3, 1, [...path, p(2, 'M')])).toBe('ата')
    expect(blood(3, 1, [...path, p(2, 'F')])).toBe('әже')
  })

  it.each<[string, PersonRef[], string]>([
    ['sister’s son, to a man', [p(1, 'M', MID), p(10, 'M'), p(11, 'F', OLD), p(2, 'M')], 'жиен'],
    ['sister’s daughter, to a woman', [p(1, 'F', MID), p(10, 'M'), p(11, 'F', YOUNG), p(2, 'F')], 'жиен'],
    ['elder brother’s son', [p(1, 'M', MID), p(10, 'M'), p(11, 'M', OLD), p(2, 'M')], 'ағасының ұлы'],
    ['younger brother’s daughter', [p(1, 'M', MID), p(10, 'M'), p(11, 'M', YOUNG), p(2, 'F')], 'інісінің қызы'],
    ['brother’s child, age unknown', [p(1, 'F', MID), p(10, 'M'), p(11, 'M'), p(2, 'U')], 'бауырының баласы'],
    ['unknown sibling’s son', [p(1, 'F', MID), p(10, 'M'), p(11, 'U', OLD), p(2, 'M')], 'бауырының ұлы'],
  ])('%s', (_, people, want) => {
    expect(blood(1, 2, people)).toBe(want)
  })

  it.each<[string, PersonRef[], string]>([
    ['cousin, elder man', [p(1, 'M', MID), father, gp, p(13, 'M'), p(2, 'M', OLD)], 'немере аға'],
    ['cousin, elder woman', [p(1, 'M', MID), father, gp, p(13, 'M'), p(2, 'F', OLD)], 'немере әпке'],
    ['cousin, younger man', [p(1, 'F', MID), father, gp, p(13, 'M'), p(2, 'M', YOUNG)], 'немере іні'],
    ['cousin, younger woman, ego man', [p(1, 'M', MID), father, gp, p(13, 'M'), p(2, 'F', YOUNG)], 'немере қарындас'],
    ['cousin, younger woman, ego woman', [p(1, 'F', MID), father, gp, p(13, 'M'), p(2, 'F', YOUNG)], 'немере сіңлі'],
    ['cousin, age unknown', [p(1, 'F'), father, gp, p(13, 'M'), p(2, 'M')], 'немере бауыр'],
  ])('%s', (_, people, want) => {
    expect(blood(2, 2, people)).toBe(want)
  })

  it.each<[string, PersonRef[], string]>([
    ['father’s brother’s child', [p(1, 'M', MID), father, gp, p(13, 'M'), p(2, 'M', OLD)], 'немере аға'],
    ['mother’s sister’s child', [p(1, 'M', MID), mother, gp, p(13, 'F'), p(2, 'M', OLD)], 'бөле'],
    ['mother’s sister’s daughter', [p(1, 'F', MID), mother, gp, p(13, 'F'), p(2, 'F', YOUNG)], 'бөле'],
    ['mother’s brother’s elder son', [p(1, 'M', MID), mother, gp, p(13, 'M'), p(2, 'M', OLD)], 'нағашы аға'],
    ['mother’s brother’s elder daughter', [p(1, 'M', MID), mother, gp, p(13, 'M'), p(2, 'F', OLD)], 'нағашы әпке'],
    ['mother’s brother’s younger son', [p(1, 'M', MID), mother, gp, p(13, 'M'), p(2, 'M', YOUNG)], 'нағашы іні'],
    ['mother’s brother’s younger daughter, ego woman', [p(1, 'F', MID), mother, gp, p(13, 'M'), p(2, 'F', YOUNG)], 'нағашы сіңлі'],
    ['father’s sister’s child', [p(1, 'M', MID), father, gp, p(13, 'F'), p(2, 'F', OLD)], 'жиен'],
    ['father’s sister’s son', [p(1, 'F', MID), father, gp, p(13, 'F'), p(2, 'M', YOUNG)], 'жиен'],
  ])('first cousin by the parents’ sexes: %s', (_, people, want) => {
    expect(blood(2, 2, people)).toBe(want)
  })

  it.each<[number, number, string]>([
    [3, 3, '6-шы буындағы туыс'],
    [4, 5, '9-шы буындағы туыс'],
    [5, 5, '10-шы буындағы туыс'],
    [7, 3, '10-шы буындағы туыс'],
    [10, 10, '20-шы буындағы туыс'],
    [15, 15, '30-шы буындағы туыс'],
    [20, 20, '40-шы буындағы туыс'],
    [25, 25, '50-ші буындағы туыс'],
    [30, 30, '60-шы буындағы туыс'],
    [35, 35, '70-ші буындағы туыс'],
    [40, 40, '80-ші буындағы туыс'],
    [45, 45, '90-шы буындағы туыс'],
    [3, 4, '7-ші буындағы туыс'],
    [2, 3, '5-ші буындағы туыс'],
    [4, 1, '5-ші буындағы туыс'],
    [1, 4, '5-ші буындағы туыс'],
  ])('very distant %i up, %i down', (up, down, want) => {
    expect(blood(up, down, [p(1, 'M'), p(2, 'M')])).toBe(want)
  })
})

describe('kk: half and adoptive', () => {
  it('uses a possessive form of the fallback term in a chain', () => {
    const rep = report('spouse_of_relative', 3, 3, [[1, 'M'], [10, 'M'], [11, 'M'], [12, 'M'], [13, 'M'], [14, 'M'], [3, 'M'], [2, 'F']], 3)
    expect(relationshipName(t, rep)).toBe('6-шы буындағы туысының әйелі')
  })

  it('ignores a birth that is only partly dated against one in the same year', () => {
    // 1980 with no month against 1980-05-01: nothing says who is older.
    expect(blood(1, 1, [p(1, 'M', 19800000), p(10, 'M'), p(2, 'M', 19800501)])).toBe('бауыр')
    expect(blood(1, 1, [p(1, 'M', 19800500), p(10, 'M'), p(2, 'M', 19800501)])).toBe('бауыр')
    expect(blood(1, 1, [p(1, 'M', 19800000), p(10, 'M'), p(2, 'M', 19790501)])).toBe('аға')
  })

  it('marks adoptive direct kin and siblings with a prefix and others with a note', () => {
    expect(blood(1, 0, [p(1, 'M'), p(2, 'M')], { adoptive: true })).toBe('асырап алған әке')
    expect(blood(1, 0, [p(1, 'M'), p(2, 'F')], { adoptive: true })).toBe('асырап алған шеше')
    expect(blood(0, 1, [p(1, 'M'), p(2, 'F')], { adoptive: true })).toBe('асыранды қыз')
    expect(blood(1, 1, [p(1, 'M', MID), p(10, 'M'), p(2, 'M', OLD)], { adoptive: true })).toBe('асыранды аға')
    expect(blood(2, 1, [p(1, 'M'), p(10, 'M'), p(12, 'M'), p(2, 'F')], { adoptive: true })).toBe('тәте (асырап алу арқылы)')
  })

  it('marks a half uncle by the shared parent', () => {
    const people = [p(1, 'M'), p(10, 'M'), p(12, 'M'), p(2, 'F'), p(20, 'M'), p(21, 'F')]
    const path = [1, 10, 12, 2]
    expect(kinshipName(t, kin(2, 1, path, { half: true, ancestors: [20] }), 'F', byId(people))).toBe('тәте (әкесі бір)')
    expect(kinshipName(t, kin(2, 1, path, { half: true, ancestors: [21] }), 'F', byId(people))).toBe('тәте (шешесі бір)')
  })
})

/**
 * An in-law report. `ids` run from A along the kinship path (for the
 * in-law kinds B is the last one and `via` is the spouse or the relative).
 */
function report(
  kind: 'spouse_of_relative' | 'relative_of_spouse',
  up: number,
  down: number,
  ids: [number, Sex, number?][],
  via: number,
  half = false,
): RelationshipReport {
  const people = ids.map(([id, sex, born]) => p(id, sex, born))
  return {
    a: ids[0]?.[0] ?? 1,
    b: people[people.length - 1]?.id ?? 2,
    kind,
    kinship: kin(up, down, ids.map(([id]) => id), { half }),
    via,
    others: [],
    persons: byId(people),
  }
}

describe('kk: in-laws', () => {
  // A is 1 (born MID), the spouse or relative via is 3, B is 2.
  const a = (sex: Sex = 'M'): [number, Sex, number] => [1, sex, MID]

  it.each<[string, RelationshipReport, string]>([
    // The spouse of a relative
    ['stepfather', report('spouse_of_relative', 1, 0, [a(), [3, 'F'], [2, 'M']], 3), 'өгей әке'],
    ['stepmother', report('spouse_of_relative', 1, 0, [a(), [3, 'M'], [2, 'F']], 3), 'өгей шеше'],
    ['son’s wife', report('spouse_of_relative', 0, 1, [a(), [3, 'M'], [2, 'F']], 3), 'келін'],
    ['daughter’s husband', report('spouse_of_relative', 0, 1, [a(), [3, 'F'], [2, 'M']], 3), 'күйеу бала'],
    ['elder sister’s husband', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'F', OLD], [2, 'M']], 3), 'жезде'],
    ['sister’s husband, age unknown', report('spouse_of_relative', 1, 1, [[1, 'M'], [10, 'M'], [3, 'F'], [2, 'M']], 3), 'жезде'],
    ['younger sister’s husband', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'F', YOUNG], [2, 'M']], 3), 'күйеу бала'],
    ['elder brother’s wife', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'M', OLD], [2, 'F']], 3), 'жеңге'],
    ['brother’s wife, age unknown', report('spouse_of_relative', 1, 1, [[1, 'F'], [10, 'M'], [3, 'M'], [2, 'F']], 3), 'жеңге'],
    ['younger brother’s wife', report('spouse_of_relative', 1, 1, [a('F'), [10, 'M'], [3, 'M', YOUNG], [2, 'F']], 3), 'келін'],
    ['brother’s husband (no term)', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'M', OLD], [2, 'M']], 3), 'ағасының күйеуі'],
    ['younger sister’s wife (no term)', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'F', YOUNG], [2, 'F']], 3), 'қарындасының әйелі'],
    ['father’s younger brother’s wife', report('spouse_of_relative', 2, 1, [a(), [10, 'M', MID], [12, 'M'], [3, 'M', YOUNG], [2, 'F']], 3), 'әкесінің інісінің әйелі'],
    ['mother’s brother’s wife', report('spouse_of_relative', 2, 1, [a(), [10, 'F', MID], [12, 'M'], [3, 'M', OLD], [2, 'F']], 3), 'нағашы ағасының әйелі'],
    ['sister’s son’s wife', report('spouse_of_relative', 1, 2, [a(), [10, 'M'], [11, 'F', OLD], [3, 'M'], [2, 'F']], 3), 'жиенінің әйелі'],
    ['unknown sibling’s spouse', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'U', OLD], [2, 'U']], 3), 'әпкесінің немесе ағасының жұбайы'],
    ['half-brother’s wife', report('spouse_of_relative', 1, 1, [a(), [10, 'M'], [3, 'M', OLD], [2, 'F']], 3, true), 'ағасының әйелі'],

    // A relative of the spouse: ids are A, the spouse, ..., B
    ['wife’s father', report('relative_of_spouse', 1, 0, [a(), [3, 'F'], [2, 'M']], 3), 'қайын ата'],
    ['husband’s mother', report('relative_of_spouse', 1, 0, [a('F'), [3, 'M'], [2, 'F']], 3), 'қайын ене'],
    ['spouse’s parent of unknown sex', report('relative_of_spouse', 1, 0, [a(), [3, 'F'], [2, 'U']], 3), 'қайын ене немесе қайын ата'],
    ['stepson', report('relative_of_spouse', 0, 1, [a(), [3, 'F'], [2, 'M']], 3), 'өгей ұл'],
    ['stepdaughter', report('relative_of_spouse', 0, 1, [a(), [3, 'F'], [2, 'F']], 3), 'өгей қыз'],
    ['stepchild', report('relative_of_spouse', 0, 1, [a(), [3, 'F'], [2, 'U']], 3), 'өгей бала'],
    ['spouse’s elder brother', report('relative_of_spouse', 1, 1, [a(), [3, 'F', MID], [10, 'M'], [2, 'M', OLD]], 3), 'қайын аға'],
    ['spouse’s younger brother', report('relative_of_spouse', 1, 1, [a(), [3, 'M', MID], [10, 'M'], [2, 'M', YOUNG]], 3), 'қайын іні'],
    ['spouse’s elder sister', report('relative_of_spouse', 1, 1, [a(), [3, 'M', MID], [10, 'M'], [2, 'F', OLD]], 3), 'қайын апа'],
    ['husband’s younger sister', report('relative_of_spouse', 1, 1, [a('F'), [3, 'M', MID], [10, 'M'], [2, 'F', YOUNG]], 3), 'қайын сіңлі'],
    ['wife’s younger sister, said by a man', report('relative_of_spouse', 1, 1, [a('M'), [3, 'F', MID], [10, 'M'], [2, 'F', YOUNG]], 3), 'балдыз'],
    ['wife’s younger sister, ego sex unknown', report('relative_of_spouse', 1, 1, [a('U'), [3, 'F', MID], [10, 'M'], [2, 'F', YOUNG]], 3), 'қайын сіңлі'],
    ['wife’s younger brother, said by a man', report('relative_of_spouse', 1, 1, [a('M'), [3, 'F', MID], [10, 'M'], [2, 'M', YOUNG]], 3), 'балдыз'],
    ['husband’s younger brother, said by a woman', report('relative_of_spouse', 1, 1, [a('F'), [3, 'M', MID], [10, 'M'], [2, 'M', YOUNG]], 3), 'қайын іні'],
    ['spouse’s sibling, age unknown', report('relative_of_spouse', 1, 1, [a(), [3, 'F'], [10, 'M'], [2, 'M']], 3), 'қайын бауыр'],
    ['spouse’s younger sibling of unknown sex', report('relative_of_spouse', 1, 1, [a(), [3, 'M', MID], [10, 'M'], [2, 'U', YOUNG]], 3), 'қайын сіңлі немесе қайын іні'],
    ['wife’s grandmother', report('relative_of_spouse', 2, 0, [a(), [3, 'F'], [10, 'M'], [2, 'F']], 3), 'әйелінің әжесі'],
    ['husband’s paternal aunt', report('relative_of_spouse', 2, 1, [a('F'), [3, 'M', MID], [10, 'M'], [12, 'M'], [2, 'F', OLD]], 3), 'күйеуінің тәтесі'],
    ['spouse’s elder brother’s child', report('relative_of_spouse', 1, 2, [a(), [3, 'M', MID], [10, 'M'], [11, 'M', OLD], [2, 'F']], 3), 'күйеуінің ағасының қызы'],
    ['parent of a spouse of unknown sex', report('relative_of_spouse', 1, 0, [a(), [3, 'U'], [2, 'M']], 3), 'қайын ата'],
    ['half-brother of the spouse', report('relative_of_spouse', 1, 1, [a(), [3, 'F', MID], [10, 'M'], [2, 'M', OLD]], 3, true), 'әйелінің ағасы (жартылай)'],
  ])('%s', (_, rep, want) => {
    expect(relationshipName(t, rep)).toBe(want)
  })

  it('handles self, spouse, blood and unrelated reports', () => {
    const base = { a: 1, b: 2, others: [], persons: { 1: p(1, 'M'), 2: p(2, 'F') } }
    expect(relationshipName(t, { ...base, kind: 'self' })).toBe(t('relationship.self'))
    expect(relationshipName(t, { ...base, kind: 'spouse' })).toBe('әйел')
    expect(relationshipName(t, { ...base, kind: 'none' })).toBeNull()
    const persons = { 1: p(1, 'M', MID), 2: p(2, 'F', YOUNG), 10: p(10, 'M') }
    expect(relationshipName(t, { ...base, kind: 'blood', kinship: kin(1, 1, [1, 10, 2]), persons })).toBe('қарындас')
  })
})
