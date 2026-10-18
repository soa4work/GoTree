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
const pluralBases = enKeys.filter((k) => k.endsWith('_one')).map((k) => k.slice(0, -4))
const categories = (locale: string) => new Intl.PluralRules(locale).resolvedOptions().pluralCategories.sort()

describe('locales', () => {
  it('kk has exactly the keys of en (kk plurals are one and other, like en)', () => {
    expect(categories('kk')).toEqual(['one', 'other'])
    expect(keys(kk).sort()).toEqual([...enKeys].sort())
  })

  it('ru has the keys of en plus few and many for every plural', () => {
    expect(categories('ru')).toEqual(['few', 'many', 'one', 'other'])
    expect(pluralBases.length).toBeGreaterThan(0)
    const extra = pluralBases.flatMap((base) => [`${base}_few`, `${base}_many`])
    expect(keys(ru).sort()).toEqual([...enKeys, ...extra].sort())
  })
})
