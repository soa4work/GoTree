import type { TFunction } from 'i18next'
import type { Kinship, PersonRef, RelationshipReport, Sex } from '../api/types'

/*
 * Russian kinship terms.
 *
 * Russian terms agree in gender and case, and many have no English-style
 * pieces ("great-", "once removed"), so they are built here from data tables
 * instead of i18n keys: a key per declined form would make hundreds of
 * strings that only this module uses, and en.json/kk.json would have to
 * carry copies of them. Only the generic words that already exist in
 * ru.json (the spouse, "or", "the same person") come from t.
 *
 * Scheme for collateral relatives. Let `up`/`down` be the generations from A
 * and B to their closest common ancestor, m = min(up, down), r = |up - down|.
 *  - Same generation (r = 0): sibling for m = 1, else the (m)-th cousin:
 *    m = 2 двоюродный, 3 троюродный, 4 четвероюродный, … up to десятиюродный.
 *  - B older (up > down): the uncle or aunt of the degree of the cousinship
 *    between A's ancestor of B's generation and B. r = 1: дядя/тётя, with
 *    "N-юродный" for N = m - 1 (so a parent's cousin is двоюродный дядя).
 *    r >= 2: дед/бабушка with пра- × (r - 2) and "N-юродный" for N = m, so
 *    that a grandparent's sibling is двоюродный дед and a grandparent's
 *    cousin is троюродный дед, as in Russian usage.
 *  - B younger (up < down): племянник/племянница for r = 1, внучатый
 *    племянник for r = 2, правнучатый for r = 3, … (пра- × (r - 2) before
 *    внучатый), with "N-юродный" for N = m - 1 in front.
 *  - Beyond десятиюродный or more than 5 generations apart, the neutral
 *    "родственник N-й степени" where N = up + down (the civil law degree).
 * Direct ancestors and descendants use пра- repeated up to four times, then
 * "предок/потомок в N-м поколении".
 */

type Gender = 'M' | 'F' | 'U'
type Case = 'nom' | 'gen'
type KinshipPart = Pick<Kinship, 'up' | 'down' | 'half' | 'adoptive'> & { ancestors?: number[] }
type Persons = Record<number, PersonRef>

const gender = (sex: Sex | undefined): Gender => (sex === 'M' || sex === 'F' ? sex : 'U')

/** Nouns by sex: nominative and genitive. */
const nouns = {
  parent: { M: ['отец', 'отца'], F: ['мать', 'матери'] },
  child: { M: ['сын', 'сына'], F: ['дочь', 'дочери'] },
  grandparent: { M: ['дед', 'деда'], F: ['бабушка', 'бабушки'] },
  grandchild: { M: ['внук', 'внука'], F: ['внучка', 'внучки'] },
  sibling: { M: ['брат', 'брата'], F: ['сестра', 'сестры'] },
  uncle: { M: ['дядя', 'дяди'], F: ['тётя', 'тёти'] },
  nephew: { M: ['племянник', 'племянника'], F: ['племянница', 'племянницы'] },
} as const
type Noun = keyof typeof nouns

/** Words for a person of unknown sex where Russian has one. */
const neutral: Partial<Record<Noun, readonly [string, string]>> = {
  parent: ['родитель', 'родителя'],
  child: ['ребёнок', 'ребёнка'],
}

const spouseGenitive = { M: 'мужа', F: 'жены', U: 'супруга(и)' } as const

const cousinAdjectives = [
  '',
  'двоюродный',
  'троюродный',
  'четвероюродный',
  'пятиюродный',
  'шестиюродный',
  'семиюродный',
  'восьмиюродный',
  'девятиюродный',
  'десятиюродный',
]
const maxCousinDegree = cousinAdjectives.length - 1
const maxRemoved = 5

/** The "N-юродный" adjective list for cousin degree n; none for n < 1. */
const cousin = (n: number): string[] => (n < 1 ? [] : [cousinAdjectives[n] ?? ''])

/** пра- × n, written out; lineal terms beyond maxPra use "предок/потомок в N-м поколении". */
const pra = (n: number) => 'пра'.repeat(Math.max(n, 0))
const maxPra = 4

/** Declines a masculine nominative adjective in -ый. */
function decline(adj: string, g: 'M' | 'F', c: Case): string {
  const paren = /^(.+) \((.+)\)$/.exec(adj)
  if (paren) return `${decline(paren[1] ?? '', g, c)} (${decline(paren[2] ?? '', g, c)})`
  const stem = adj.slice(0, -2)
  if (c === 'nom') return g === 'M' ? adj : `${stem}ая`
  return `${stem}${g === 'M' ? 'ого' : 'ой'}`
}

interface Term {
  /** Masculine nominative adjectives, in order. */
  adj: string[]
  noun: Noun
  /** Prefix of the noun: пра- for great-grandparents. */
  pra?: string
  /** Text after the noun. */
  tail?: string
}

function render(t: TFunction, term: Term, g: Gender, c: Case): string {
  const one = (x: 'M' | 'F', useNeutral: boolean) => {
    const words = term.adj.map((a) => decline(a, x, c))
    const noun = (useNeutral && neutral[term.noun]) || nouns[term.noun][x]
    return [...words, (term.pra ?? '') + noun[c === 'nom' ? 0 : 1]].join(' ') + (term.tail ?? '')
  }
  if (g !== 'U') return one(g, false)
  if (neutral[term.noun] && !term.pra) return one('M', true)
  return t('relationship.either', { a: one('F', false), b: one('M', false) })
}

/** The term for a blood kinship (what B is to A), or null when too distant. */
function bloodTerm(k: KinshipPart, halfSex: Sex | undefined): Term | null {
  const { up, down } = k
  let term: Term
  if (down === 0) {
    term = up === 1 ? { adj: [], noun: 'parent' } : { adj: [], noun: 'grandparent', pra: pra(up - 2) }
  } else if (up === 0) {
    term = down === 1 ? { adj: [], noun: 'child' } : { adj: [], noun: 'grandchild', pra: pra(down - 2) }
  } else {
    const m = Math.min(up, down)
    const r = Math.abs(up - down)
    if (r > maxRemoved) return null
    if (r === 0) {
      if (m - 1 > maxCousinDegree) return null
      term = { adj: cousin(m - 1), noun: 'sibling' }
    } else if (up > down) {
      const degree = r === 1 ? m - 1 : m
      if (degree > maxCousinDegree) return null
      const adj = cousin(degree)
      term = r === 1 ? { adj, noun: 'uncle' } : { adj, noun: 'grandparent', pra: pra(r - 2) }
    } else {
      if (m - 1 > maxCousinDegree) return null
      const adj = cousin(m - 1)
      if (r >= 2) adj.push(`${pra(r - 2)}внучатый`)
      term = { adj, noun: 'nephew' }
    }
  }
  return modify(term, k, halfSex)
}

/** Half-blood and adoptive marks on a term. */
function modify(term: Term, k: KinshipPart, halfSex: Sex | undefined): Term {
  const lineal = (k.up === 1 && k.down === 0) || (k.up === 0 && k.down === 1)
  const sibling = k.up === 1 && k.down === 1
  if (k.adoptive) {
    // The graph cannot tell an adopted child from a step-child: direct kin
    // are "приёмные", siblings joined by a non-birth link are named both ways.
    if (lineal) return { ...term, adj: ['приёмный'] }
    if (sibling) return { ...term, adj: ['сводный (приёмный)'] }
    return { ...term, tail: ' (не по рождению)' }
  }
  if (k.half) {
    // Same father: единокровный; same mother: единоутробный.
    if (sibling) return { ...term, adj: [halfSex === 'M' ? 'единокровный' : halfSex === 'F' ? 'единоутробный' : 'неполнородный'] }
    if (halfSex === 'M') return { ...term, tail: ' (по отцу)' }
    if (halfSex === 'F') return { ...term, tail: ' (по матери)' }
    return { ...term, adj: ['неполнородный', ...term.adj] }
  }
  return term
}

function distant(k: KinshipPart, g: Gender, c: Case): string {
  const man = c === 'nom' ? 'родственник' : 'родственника'
  const woman = c === 'nom' ? 'родственница' : 'родственницы'
  const who = g === 'M' ? man : g === 'F' ? woman : `${man} или ${woman}`
  return `${who} ${k.up + k.down}-й степени`
}

function kinship(t: TFunction, k: KinshipPart, sex: Sex | undefined, persons: Persons | undefined, c: Case): string {
  const g = gender(sex)
  const halfSex = k.half && k.ancestors?.[0] ? persons?.[k.ancestors[0]]?.sex : undefined
  if ((k.down === 0 && k.up - 2 > maxPra) || (k.up === 0 && k.down - 2 > maxPra)) {
    const [nom, gen, n] = k.down === 0 ? ['предок', 'предка', k.up] : ['потомок', 'потомка', k.down]
    return `${c === 'nom' ? nom : gen} в ${n}-м поколении`
  }
  const term = bloodTerm(k, halfSex)
  return term ? render(t, term, g, c) : distant(k, g, c)
}

/** What someone of sex `sex` is to the other person, by a blood kinship. */
export function kinshipName(t: TFunction, k: KinshipPart, sex: Sex | undefined, persons?: Persons): string {
  return kinship(t, k, sex, persons, 'nom')
}

const spouseTerm = (t: TFunction, sex: Sex | undefined) => t(`relationship.spouse_${gender(sex)}` as 'relationship.spouse_U')

const is = (k: KinshipPart, up: number, down: number) => k.up === up && k.down === down

/** A word for a person of sex g, or "F or M" when unknown. */
function pick(t: TFunction, g: Gender, m: string, f: string): string {
  return g === 'M' ? m : g === 'F' ? f : t('relationship.either', { a: f, b: m })
}

/**
 * What B is to A, e.g. "двоюродный брат" or "свекровь"; null when they are
 * not related. In-laws depend on whose spouse the link goes through: via is
 * the spouse (husband's or wife's side), or the relative whose spouse B is.
 * Without a special term the name is a chain: "жена дяди", "брат мужа".
 */
export function relationshipName(t: TFunction, rep: RelationshipReport): string | null {
  const a = rep.persons[rep.a]
  const b = rep.persons[rep.b]
  const via = rep.via ? rep.persons[rep.via] : undefined
  const k = rep.kinship
  const g = gender(b?.sex)
  const viaG = gender(via?.sex)
  switch (rep.kind) {
    case 'self':
      return t('relationship.self')
    case 'spouse':
      return spouseTerm(t, b?.sex)
    case 'blood':
      return k ? kinship(t, k, b?.sex, rep.persons, 'nom') : null
    case 'spouse_of_relative': {
      if (!k) return null
      if (is(k, 1, 0)) return pick(t, g, 'отчим', 'мачеха')
      const childInLaw = is(k, 0, 1) || (is(k, 1, 1) && !k.half)
      if (childInLaw && g === 'F' && viaG === 'M') {
        // Son's wife: сноха to the husband's father, невестка otherwise; a brother's wife is невестка.
        return is(k, 0, 1) && gender(a?.sex) === 'M' ? 'сноха' : 'невестка'
      }
      if (childInLaw && g === 'M' && viaG === 'F') return 'зять'
      return `${spouseTerm(t, b?.sex)} ${kinship(t, k, via?.sex, rep.persons, 'gen')}`
    }
    case 'relative_of_spouse': {
      if (!k) return null
      if (is(k, 1, 0) && viaG !== 'U') {
        // Husband's parents: свёкор, свекровь; wife's parents: тесть, тёща.
        return viaG === 'M' ? pick(t, g, 'свёкор', 'свекровь') : pick(t, g, 'тесть', 'тёща')
      }
      if (is(k, 0, 1)) return pick(t, g, 'пасынок', 'падчерица')
      if (is(k, 1, 1) && !k.half && viaG !== 'U' && g !== 'U') {
        // Husband's brother, sister: деверь, золовка; wife's: шурин, свояченица.
        return viaG === 'M' ? pick(t, g, 'деверь', 'золовка') : pick(t, g, 'шурин', 'свояченица')
      }
      return `${kinship(t, k, b?.sex, rep.persons, 'nom')} ${spouseGenitive[viaG]}`
    }
    default:
      return null
  }
}
