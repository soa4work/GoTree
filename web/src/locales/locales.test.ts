import { describe, expect, it } from 'vitest'
import en from './en.json'
import kk from './kk.json'
import ru from './ru.json'

function keys(tree: object, prefix = ''): string[] {
  return Object.entries(tree).flatMap(([k, v]) =>
    typeof v === 'object' && v !== null ? keys(v, `${prefix}${k}.`) : [`${prefix}${k}`],
  )
}

const enKeys = keys(en)

describe('locales', () => {
  it.each([
    ['ru', ru],
    ['kk', kk],
  ])('%s has exactly the keys of en, except plural forms', (name, tree) => {
    const strip = (list: string[]) => list.filter((k) => !/_(zero|one|two|few|many|other)$/.test(k))
    expect(strip(keys(tree)).sort(), name).toEqual(strip(enKeys).sort())
  })

  it('kk uses the plural forms of en (one, other)', () => {
    expect(new Intl.PluralRules('kk').resolvedOptions().pluralCategories.sort()).toEqual(['one', 'other'])
    expect(keys(kk).sort()).toEqual([...enKeys].sort())
  })

  it('ru has one, few, many and other for every plural of en', () => {
    const ruKeys = new Set(keys(ru))
    expect(new Intl.PluralRules('ru').resolvedOptions().pluralCategories.sort()).toEqual(['few', 'many', 'one', 'other'])
    const bases = enKeys.filter((k) => k.endsWith('_one')).map((k) => k.slice(0, -4))
    expect(bases.length).toBeGreaterThan(0)
    for (const base of bases) {
      for (const form of ['one', 'few', 'many', 'other']) expect(ruKeys.has(`${base}_${form}`), `${base}_${form}`).toBe(true)
    }
    const expected = enKeys.flatMap((k) => (k.endsWith('_one') ? [k, ...['few', 'many'].map((f) => `${k.slice(0, -4)}_${f}`)] : [k]))
    expect([...ruKeys].sort()).toEqual([...new Set(expected)].sort())
  })
})
