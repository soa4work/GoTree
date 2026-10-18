import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import type { PersonRef, RelationshipReport, Sex } from '../api/types'
import i18n from '../i18n'
import { kinshipName, relationshipName } from './name'

const t = i18n.t.bind(i18n)
let previous = i18n.language

beforeAll(async () => {
  previous = i18n.language
  await i18n.changeLanguage('ru')
})
afterAll(async () => {
  await i18n.changeLanguage(previous)
})

const k = (up: number, down: number, extra: { half?: boolean; adoptive?: boolean; ancestors?: number[] } = {}) => ({
  up,
  down,
  half: false,
  adoptive: false,
  ...extra,
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

describe('ru: lineal kin', () => {
  it.each<[number, number, Sex, string]>([
    [1, 0, 'M', 'отец'],
    [1, 0, 'F', 'мать'],
    [1, 0, 'U', 'родитель'],
    [2, 0, 'M', 'дед'],
    [2, 0, 'F', 'бабушка'],
    [2, 0, 'U', 'бабушка или дед'],
    [3, 0, 'M', 'прадед'],
    [3, 0, 'F', 'прабабушка'],
    [4, 0, 'M', 'прапрадед'],
    [4, 0, 'F', 'прапрабабушка'],
    [5, 0, 'M', 'прапрапрадед'],
    [6, 0, 'M', 'пра×4-дед'],
    [7, 0, 'F', 'пра×5-бабушка'],
    [0, 1, 'M', 'сын'],
    [0, 1, 'F', 'дочь'],
    [0, 1, 'U', 'ребёнок'],
    [0, 2, 'M', 'внук'],
    [0, 2, 'F', 'внучка'],
    [0, 3, 'M', 'правнук'],
    [0, 3, 'F', 'правнучка'],
    [0, 4, 'M', 'праправнук'],
    [0, 4, 'F', 'праправнучка'],
    [0, 6, 'M', 'пра×4-внук'],
  ])('%i up, %i down, sex %s: %s', (up, down, sex, want) => {
    expect(kinshipName(t, k(up, down), sex)).toBe(want)
  })
})

describe('ru: siblings, uncles and nephews', () => {
  it.each<[number, number, Sex, string]>([
    [1, 1, 'M', 'брат'],
    [1, 1, 'F', 'сестра'],
    [1, 1, 'U', 'сестра или брат'],
    [2, 1, 'M', 'дядя'],
    [2, 1, 'F', 'тётя'],
    [2, 1, 'U', 'тётя или дядя'],
    [3, 1, 'M', 'двоюродный дед'],
    [3, 1, 'F', 'двоюродная бабушка'],
    [4, 1, 'M', 'двоюродный прадед'],
    [4, 1, 'F', 'двоюродная прабабушка'],
    [1, 2, 'M', 'племянник'],
    [1, 2, 'F', 'племянница'],
    [1, 2, 'U', 'племянница или племянник'],
    [1, 3, 'M', 'внучатый племянник'],
    [1, 3, 'F', 'внучатая племянница'],
    [1, 4, 'M', 'правнучатый племянник'],
    [1, 4, 'F', 'правнучатая племянница'],
    [1, 5, 'M', 'праправнучатый племянник'],
  ])('%i up, %i down, sex %s: %s', (up, down, sex, want) => {
    expect(kinshipName(t, k(up, down), sex)).toBe(want)
  })
})

describe('ru: cousins', () => {
  it.each<[number, number, Sex, string]>([
    [2, 2, 'M', 'двоюродный брат'],
    [2, 2, 'F', 'двоюродная сестра'],
    [3, 3, 'M', 'троюродный брат'],
    [3, 3, 'F', 'троюродная сестра'],
    [4, 4, 'M', 'четвероюродный брат'],
    [5, 5, 'F', 'пятиюродная сестра'],
    [6, 6, 'M', 'шестиюродный брат'],
    [9, 9, 'F', 'девятиюродная сестра'],
    [10, 10, 'M', 'десятиюродный брат'],
    // Removed cousins: the cousin of a parent, grandparent, child, grandchild.
    [3, 2, 'M', 'двоюродный дядя'],
    [3, 2, 'F', 'двоюродная тётя'],
    [4, 3, 'M', 'троюродный дядя'],
    [4, 2, 'M', 'троюродный дед'],
    [4, 2, 'F', 'троюродная бабушка'],
    [5, 2, 'M', 'троюродный прадед'],
    [2, 3, 'M', 'двоюродный племянник'],
    [2, 3, 'F', 'двоюродная племянница'],
    [3, 4, 'M', 'троюродный племянник'],
    [2, 4, 'M', 'двоюродный внучатый племянник'],
    [2, 4, 'F', 'двоюродная внучатая племянница'],
    [2, 5, 'F', 'двоюродная правнучатая племянница'],
    [2, 2, 'U', 'двоюродная сестра или двоюродный брат'],
  ])('%i up, %i down, sex %s: %s', (up, down, sex, want) => {
    expect(kinshipName(t, k(up, down), sex)).toBe(want)
  })

  it.each<[number, number, Sex, string]>([
    [11, 11, 'M', 'родственник 22-й степени'],
    [12, 12, 'M', 'родственник 24-й степени'],
    [12, 12, 'F', 'родственница 24-й степени'],
    [12, 12, 'U', 'родственник или родственница 24-й степени'],
    [2, 9, 'F', 'родственница 11-й степени'],
    [8, 1, 'M', 'родственник 9-й степени'],
  ])('very distant %i up, %i down, sex %s: %s', (up, down, sex, want) => {
    expect(kinshipName(t, k(up, down), sex)).toBe(want)
  })
})

describe('ru: half, step and adoptive', () => {
  const persons = { 10: person(10, 'M'), 11: person(11, 'F') }

  it.each<[string, Sex, number, string]>([
    ['same father', 'M', 10, 'единокровный брат'],
    ['same father', 'F', 10, 'единокровная сестра'],
    ['same mother', 'M', 11, 'единоутробный брат'],
    ['same mother', 'F', 11, 'единоутробная сестра'],
    ['shared parent unknown', 'M', -3, 'неполнородный брат'],
  ])('half-sibling, %s, B is %s', (_, sex, ancestor, want) => {
    expect(kinshipName(t, k(1, 1, { half: true, ancestors: [ancestor] }), sex, persons)).toBe(want)
  })

  it('names half uncles without a special word', () => {
    expect(kinshipName(t, k(2, 1, { half: true, ancestors: [10] }), 'M', persons)).toBe('дядя (по одному из родителей)')
  })

  it('marks adoptive and step links', () => {
    expect(kinshipName(t, k(1, 0, { adoptive: true }), 'M')).toBe('приёмный отец')
    expect(kinshipName(t, k(1, 0, { adoptive: true }), 'F')).toBe('приёмная мать')
    expect(kinshipName(t, k(0, 1, { adoptive: true }), 'F')).toBe('приёмная дочь')
    expect(kinshipName(t, k(1, 1, { adoptive: true }), 'M')).toBe('сводный брат')
    expect(kinshipName(t, k(1, 1, { adoptive: true }), 'F')).toBe('сводная сестра')
    expect(kinshipName(t, k(2, 1, { adoptive: true }), 'F')).toBe('тётя (не по рождению)')
  })
})

function report(
  kind: RelationshipReport['kind'],
  up: number,
  down: number,
  bSex: Sex,
  viaSex: Sex = 'U',
  aSex: Sex = 'U',
  extra: { half?: boolean; ancestors?: number[] } = {},
): RelationshipReport {
  return {
    a: 1,
    b: 2,
    kind,
    kinship: { ...k(up, down, extra), ancestors: extra.ancestors ?? [], path: [] },
    via: 3,
    others: [],
    persons: { 1: person(1, aSex), 2: person(2, bSex), 3: person(3, viaSex), 10: person(10, 'M'), 11: person(11, 'F') },
  }
}

describe('ru: relationship names', () => {
  it.each<[string, RelationshipReport, string | null]>([
    ['self', { ...report('self', 0, 0, 'U'), kinship: undefined }, 'тот же человек'],
    ['husband', { ...report('spouse', 0, 0, 'M'), kinship: undefined }, 'муж'],
    ['wife', { ...report('spouse', 0, 0, 'F'), kinship: undefined }, 'жена'],
    ['not related', { ...report('none', 0, 0, 'U'), kinship: undefined }, null],
    ['blood cousin', report('blood', 2, 2, 'F'), 'двоюродная сестра'],
    ['blood half-brother by father', report('blood', 1, 1, 'M', 'U', 'U', { half: true, ancestors: [10] }), 'единокровный брат'],
    ['blood half-sister by mother', report('blood', 1, 1, 'F', 'U', 'U', { half: true, ancestors: [11] }), 'единоутробная сестра'],

    // A's relative's spouse
    ['stepfather', report('spouse_of_relative', 1, 0, 'M'), 'отчим'],
    ['stepmother', report('spouse_of_relative', 1, 0, 'F'), 'мачеха'],
    ['stepparent', report('spouse_of_relative', 1, 0, 'U'), 'мачеха или отчим'],
    ['son-in-law', report('spouse_of_relative', 0, 1, 'M', 'F', 'F'), 'зять'],
    ['son’s wife to the mother', report('spouse_of_relative', 0, 1, 'F', 'M', 'F'), 'невестка'],
    ['son’s wife to the father', report('spouse_of_relative', 0, 1, 'F', 'M', 'M'), 'сноха'],
    ['son’s wife, ego unknown', report('spouse_of_relative', 0, 1, 'F', 'M'), 'невестка'],
    ['daughter’s wife', report('spouse_of_relative', 0, 1, 'F', 'F'), 'жена дочери'],
    ['son’s husband', report('spouse_of_relative', 0, 1, 'M', 'M'), 'муж сына'],
    ['child’s spouse of unknown sex', report('spouse_of_relative', 0, 1, 'U', 'F'), 'супруг(а) дочери'],
    ['sister’s husband', report('spouse_of_relative', 1, 1, 'M', 'F'), 'зять'],
    ['brother’s wife', report('spouse_of_relative', 1, 1, 'F', 'M'), 'невестка'],
    ['brother’s husband', report('spouse_of_relative', 1, 1, 'M', 'M'), 'муж брата'],
    ['sister’s wife', report('spouse_of_relative', 1, 1, 'F', 'F'), 'жена сестры'],
    ['uncle’s wife', report('spouse_of_relative', 2, 1, 'F', 'M'), 'жена дяди'],
    ['first cousin’s husband', report('spouse_of_relative', 2, 2, 'M', 'F'), 'муж двоюродной сестры'],
    ['half-brother’s wife', report('spouse_of_relative', 1, 1, 'F', 'M', 'U', { half: true, ancestors: [10] }), 'жена единокровного брата'],

    // A's spouse's relative: the husband's side is via a man, the wife's via a woman
    ['husband’s father', report('relative_of_spouse', 1, 0, 'M', 'M'), 'свёкор'],
    ['husband’s mother', report('relative_of_spouse', 1, 0, 'F', 'M'), 'свекровь'],
    ['wife’s father', report('relative_of_spouse', 1, 0, 'M', 'F'), 'тесть'],
    ['wife’s mother', report('relative_of_spouse', 1, 0, 'F', 'F'), 'тёща'],
    ['spouse’s father, spouse sex unknown', report('relative_of_spouse', 1, 0, 'M'), 'отец супруга(и)'],
    ['stepson', report('relative_of_spouse', 0, 1, 'M', 'F'), 'пасынок'],
    ['stepdaughter', report('relative_of_spouse', 0, 1, 'F', 'M'), 'падчерица'],
    ['husband’s brother', report('relative_of_spouse', 1, 1, 'M', 'M'), 'деверь'],
    ['husband’s sister', report('relative_of_spouse', 1, 1, 'F', 'M'), 'золовка'],
    ['wife’s brother', report('relative_of_spouse', 1, 1, 'M', 'F'), 'шурин'],
    ['wife’s sister', report('relative_of_spouse', 1, 1, 'F', 'F'), 'свояченица'],
    ['spouse’s sibling, unknown sex', report('relative_of_spouse', 1, 1, 'U', 'F'), 'сестра или брат жены'],
    ['spouse’s sister, spouse unknown', report('relative_of_spouse', 1, 1, 'F'), 'сестра супруга(и)'],
    ['husband’s half-sister', report('relative_of_spouse', 1, 1, 'F', 'M', 'U', { half: true, ancestors: [11] }), 'единоутробная сестра мужа'],
    ['wife’s first cousin', report('relative_of_spouse', 2, 2, 'M', 'F'), 'двоюродный брат жены'],
    ['husband’s grandmother', report('relative_of_spouse', 2, 0, 'F', 'M'), 'бабушка мужа'],
    ['wife’s aunt', report('relative_of_spouse', 2, 1, 'F', 'F'), 'тётя жены'],
    ['wife’s very distant relative', report('relative_of_spouse', 12, 12, 'F', 'F'), 'родственница 24-й степени жены'],
  ])('%s', (_, rep, want) => {
    expect(relationshipName(t, rep)).toBe(want)
  })
})

describe('language dispatch', () => {
  it('keeps the English pieces for English and Kazakh, and Russian terms only for ru', async () => {
    expect(kinshipName(t, k(2, 2), 'M')).toBe('двоюродный брат')
    await i18n.changeLanguage('en')
    expect(kinshipName(t, k(2, 2), 'M')).toBe('first cousin')
    await i18n.changeLanguage('kk')
    expect(kinshipName(t, k(2, 0), 'F')).toBe('әже')
    await i18n.changeLanguage('ru')
  })
})
