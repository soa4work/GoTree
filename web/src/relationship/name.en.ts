import type { TFunction } from 'i18next'
import type { Kinship, RelationshipReport, Sex } from '../api/types'

type Gender = 'M' | 'F' | 'U'

const gender = (sex: Sex | undefined): Gender => (sex === 'M' || sex === 'F' ? sex : 'U')

/**
 * Builds a term for a gender. Terms without a neutral word ("uncle") are
 * given for unknown sex as "aunt or uncle", with any prefix applied to
 * both halves.
 */
function gendered(t: TFunction, g: Gender, build: (g: 'M' | 'F' | 'U') => string, neutral: boolean): string {
  if (g !== 'U' || neutral) return build(g)
  return t('relationship.either', { a: build('F'), b: build('M') })
}

function greats(t: TFunction, n: number): string {
  if (n <= 0) return ''
  if (n <= 2) return t('relationship.great').repeat(n)
  return t('relationship.greatTimes', { count: n })
}

function ordinal(t: TFunction, n: number): string {
  return n <= 8 ? t(`relationship.ordinal${n}` as 'relationship.ordinal1') : t('relationship.ordinalN', { count: n })
}

/** What someone of sex `sex` is to the other person, by a blood kinship. */
export function kinshipName(
  t: TFunction,
  k: Pick<Kinship, 'up' | 'down' | 'half' | 'adoptive'>,
  sex: Sex | undefined,
): string {
  const g = gender(sex)
  const half = k.half ? t('relationship.half') : ''
  const term = (key: string, x: Gender) => t(`relationship.${key}_${x}` as 'relationship.parent_U')
  let name: string
  if (k.down === 0) {
    name =
      k.up === 1
        ? term('parent', g)
        : gendered(t, g, (x) => greats(t, k.up - 2) + term('grandparent', x), true)
  } else if (k.up === 0) {
    name = k.down === 1 ? term('child', g) : gendered(t, g, (x) => greats(t, k.down - 2) + term('grandchild', x), true)
  } else if (k.up === 1 && k.down === 1) {
    name = half + term('sibling', g)
  } else if (k.up === 1) {
    const grand = k.down >= 3 ? greats(t, k.down - 3) + t('relationship.grand') : ''
    name = gendered(t, g, (x) => half + grand + term('nephew', x), false)
  } else if (k.down === 1) {
    name = gendered(t, g, (x) => half + greats(t, k.up - 2) + term('uncle', x), false)
  } else {
    const degree = Math.min(k.up, k.down) - 1
    const removed = Math.abs(k.up - k.down)
    name = half + t('relationship.cousin', { ordinal: ordinal(t, degree) })
    if (removed > 0) name = t('relationship.removed', { name, count: removed })
  }
  return k.adoptive ? t('relationship.adoptive', { name }) : name
}

const spouseTerm = (t: TFunction, sex: Sex | undefined) =>
  t(`relationship.spouse_${gender(sex)}` as 'relationship.spouse_U')

const is = (k: Kinship, up: number, down: number) => k.up === up && k.down === down

/**
 * What B is to A, e.g. "first cousin once removed" or "sister-in-law";
 * null when they are not related.
 */
export function relationshipName(t: TFunction, rep: RelationshipReport): string | null {
  const b = rep.persons[rep.b]
  const via = rep.via ? rep.persons[rep.via] : undefined
  const k = rep.kinship
  const g = gender(b?.sex)
  const term = (key: string) => t(`relationship.${key}_${g}` as 'relationship.parent_U')
  switch (rep.kind) {
    case 'self':
      return t('relationship.self')
    case 'spouse':
      return spouseTerm(t, b?.sex)
    case 'blood':
      return k ? kinshipName(t, k, b?.sex) : null
    case 'spouse_of_relative':
      if (!k) return null
      if (is(k, 1, 0)) return term('stepParent')
      if (is(k, 0, 1)) return term('childInLaw')
      if (is(k, 1, 1) && !k.half) return term('siblingInLaw')
      return t('relationship.possessive', { owner: kinshipName(t, k, via?.sex), name: spouseTerm(t, b?.sex) })
    case 'relative_of_spouse':
      if (!k) return null
      if (is(k, 1, 0)) return term('parentInLaw')
      if (is(k, 0, 1)) return term('stepChild')
      if (is(k, 1, 1) && !k.half) return term('siblingInLaw')
      return t('relationship.possessive', { owner: spouseTerm(t, via?.sex), name: kinshipName(t, k, b?.sex) })
    default:
      return null
  }
}
