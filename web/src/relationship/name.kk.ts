import type { TFunction } from 'i18next'
import type { Kinship, PersonRef, RelationshipReport, Sex } from '../api/types'

/*
 * Kazakh kinship terms (to be proofread by a native speaker).
 *
 * Kazakh names relatives by seniority and by side, not by generations and
 * degrees, and its terms take possessive and case endings, so they are built
 * here from data instead of i18n keys. Only the generic words that exist in
 * kk.json (the spouse, "or", "the same person") come from t. Chains of terms
 * ("ағасының әйелі") carry the endings on the terms; names of people never
 * get any.
 *
 * Seniority compares the birth sort keys (PersonRef.birthSort, YYYYMMDD) of
 * the two people; when either is missing the term for an unknown age is used.
 * The ego (the person the term is said from) is the first person of the blood
 * path, which is why kinship parts carry `path`.
 *
 * Direct kin
 *  - Parents: әке, шеше; unknown sex ата-ана. Grandparents: ата, әже through the
 *    father (or when the side is unknown), нағашы ата, нағашы әже through the
 *    mother. Great-grandparents: арғы ата, арғы әже. Further up
 *    "N-ші буын ата-бабасы".
 *  - Children: ұл, қыз, бала. Grandchildren: немере, but жиен through a
 *    daughter. Then шөбере, шөпшек, немене; further "N-ші буын ұрпағы".
 * Siblings (B by age and sex, ego's sex only for the younger sister)
 *  - elder: аға, әпке; younger: іні, and қарындас (ego male) or сіңлі (ego
 *    female). Unknown age: бауыр, a sister қыз бауыр.
 * Collateral
 *  - Father's elder brother көке («үлкен әке» is the more standard variant, көке
 *    is kept as the shorter one), younger "әкесінің інісі" (unknown age
 *    "әкесінің бауыры"), father's sister тәте; mother's brother нағашы аға,
 *    mother's sister нағашы апа. Parent's sibling through an unknown parent:
 *    "ата-анасының ағасы" and so on. A grandparent's sibling is ата or әже.
 *  - Sister's child: жиен; brother's child: "ағасының ұлы" (by his age against
 *    ego), "інісінің қызы", "бауырының баласы" when it is unknown.
 *  - First cousins by the parents' sexes: through the father's brother "немере"
 *    + the sibling term (немере аға, немере сіңлі); through the mother's
 *    sister бөле; through the mother's brother "нағашы" + the sibling term
 *    (нағашы аға, нағашы сіңлі); through the father's sister жиен. With an
 *    unknown parent sex, "немере" + the sibling term.
 *  - Anything else: "N-ші буындағы туыс" with N = up + down; ordinals take
 *    -ші or -шы by the vowel harmony of the numeral (6-шы, 9-шы, 10-шы).
 * Half-blood marks: "(әкесі бір)" (same father), "(шешесі бір)" (same mother).
 * Adoptive parents are "асырап алған әке/шеше", adoptive children and siblings
 * take the prefix асыранды.
 *
 * In-laws (by the side the link goes through)
 *  - spouse's parents: қайын ата, қайын ене; step-parent: өгей әке, өгей шеше;
 *    step-child: өгей ұл, өгей қыз, өгей бала.
 *  - spouse's siblings: қайын аға, қайын іні, қайын апа, қайын сіңлі by age,
 *    балдыз for a wife's younger sibling of either sex (said by a man), қайын бауыр when age
 *    is unknown.
 *  - sister's husband: жезде (elder or unknown age), күйеу бала (younger);
 *    daughter's husband: күйеу бала. Brother's wife: жеңге (elder or unknown
 *    age), келін (younger); son's wife: келін.
 *  - Everything else is a chain: "ағасының әйелі", "әйелінің ағасы".
 *  - Not expressible with the graph (it has no two-step links): абысын, бажа,
 *    құда, құдағи.
 */

type Gender = 'M' | 'F' | 'U'
type Rank = 'elder' | 'younger' | undefined
type KinshipPart = Pick<Kinship, 'up' | 'down' | 'half' | 'adoptive'> & { ancestors?: number[]; path?: number[] }
type Persons = Record<number, PersonRef>

const gender = (sex: Sex | undefined): Gender => (sex === 'M' || sex === 'F' ? sex : 'U')

/** One way to say it: the plain form and the form a following noun can be owned by. */
interface Alt {
  nom: string
  /** Third person possessive ("ағасы"), the base of a genitive in a chain. */
  poss: string
  /** A half-blood note after the term, which an owner form leaves out. */
  mark?: string
}
/** Alternatives for an unknown sex, in the order they are listed ("әпке немесе аға"). */
type Term = Alt[]

const possessive: Record<string, string> = {
  аға: 'ағасы',
  іні: 'інісі',
  әпке: 'әпкесі',
  қарындас: 'қарындасы',
  сіңлі: 'сіңлісі',
  бауыр: 'бауыры',
  бөле: 'бөлесі',
  туыс: 'туысы',
  әке: 'әкесі',
  шеше: 'шешесі',
  'ата-ана': 'ата-анасы',
  ата: 'атасы',
  әже: 'әжесі',
  ұл: 'ұлы',
  қыз: 'қызы',
  бала: 'баласы',
  немере: 'немересі',
  жиен: 'жиені',
  шөбере: 'шөбересі',
  шөпшек: 'шөпшегі',
  немене: 'неменесі',
  көке: 'көкесі',
  тәте: 'тәтесі',
  апа: 'апасы',
}

/** A term of one or more words whose last word is a head noun from the table. */
function simple(nom: string): Alt {
  const words = nom.split(' ')
  const head = words.pop() ?? ''
  return { nom, poss: [...words, possessive[head] ?? head].join(' ') }
}
/** A phrase that already ends in a possessive form. */
const phrase = (nom: string): Alt => ({ nom, poss: nom })

const one = (nom: string): Term => [simple(nom)]
const many = (...noms: string[]): Term => noms.map(simple)

const backVowels = 'аоұы'
const frontVowels = 'әеіөү'

/** Genitive of a possessive form: ағасы -> ағасының, әкесі -> әкесінің. */
function genitive(word: string): string {
  const vowels = [...word].filter((c) => backVowels.includes(c) || frontVowels.includes(c))
  const last = vowels[vowels.length - 1] ?? 'а'
  return word + (backVowels.includes(last) ? 'ның' : 'нің')
}

/** Alternatives joined with "or". */
function join(t: TFunction, words: string[]): string {
  return words.reduce((acc, w) => t('relationship.either', { a: acc, b: w }))
}
function render(t: TFunction, term: Term): string {
  return join(t, term.map((a) => a.nom + (a.mark ?? '')))
}
/** The term as a possessed noun, for a chain's head: "әйелінің ағасы". */
function renderPossessed(t: TFunction, term: Term): string {
  return join(t, term.map((a) => a.poss + (a.mark ?? '')))
}

/** "<owner>'s <head>": the owner's possessive form in the genitive, then the head. */
function chain(t: TFunction, owner: Term, head: string): string {
  return `${join(t, owner.map((a) => genitive(a.poss)))} ${head}`
}

const withPrefix = (term: Term, prefix: string): Term => term.map((a) => ({ nom: `${prefix} ${a.nom}`, poss: `${prefix} ${a.poss}` }))
/** Marks the plain form only: an owner in a chain stays unmarked. */
const withTail = (term: Term, tail: string): Term => term.map((a) => ({ ...a, mark: tail }))

/** Whether B (born `b`) is older or younger than `a`; undefined when either birth is unknown. */
function rankOf(b: PersonRef | undefined, a: PersonRef | undefined): Rank {
  const x = b?.birthSort
  const y = a?.birthSort
  if (!x || !y) return undefined
  // A date with a 00 month or day is partial: compare only what both state.
  const precision = (v: number) => (Math.floor(v / 10000) === 0 ? 0 : Math.floor(v / 100) % 100 === 0 ? 4 : v % 100 === 0 ? 6 : 8)
  const digits = Math.min(precision(x), precision(y))
  const cut = (v: number) => Math.floor(v / 10 ** (8 - digits))
  if (cut(x) === cut(y)) return undefined
  return cut(x) < cut(y) ? 'elder' : 'younger'
}

/** The sibling term of someone of sex g by rank against ego of sex `ego`. */
function sibling(g: Gender, rank: Rank, ego: Gender): Term {
  if (rank === 'elder') return g === 'M' ? one('аға') : g === 'F' ? one('әпке') : many('әпке', 'аға')
  if (rank === 'younger') {
    const sister = ego === 'M' ? ['қарындас'] : ego === 'F' ? ['сіңлі'] : ['қарындас', 'сіңлі']
    return g === 'M' ? one('іні') : g === 'F' ? many(...sister) : many(...sister, 'іні')
  }
  return g === 'F' ? one('қыз бауыр') : one('бауыр')
}

/** Picks the term of each sex for B and joins both when B's sex is unknown (female first). */
function bySex(g: Gender, male: () => Term, female: () => Term): Term {
  return g === 'M' ? male() : g === 'F' ? female() : [...female(), ...male()]
}

/** The written ordinal: the suffix is -шы after a numeral ending in a back vowel (алтыншы, тоғызыншы, оныншы), else -ші. */
function ordinal(n: number): string {
  const unit = n % 10
  const tens = (n % 100) - unit
  let back: boolean
  if (unit) back = unit === 6 || unit === 9
  else if (tens) back = [10, 20, 30, 40, 60, 90].includes(tens)
  else back = n % 1000 === 0
  return `${n}-${back ? 'шы' : 'ші'}`
}

interface Context {
  /** The ego: the person the term is said from. */
  ego: PersonRef | undefined
  /** The people on the blood path. */
  at: (i: number) => PersonRef | undefined
}

function context(k: KinshipPart, persons: Persons | undefined): Context {
  const at = (i: number) => {
    const id = k.path?.[i]
    return id !== undefined && id > 0 ? persons?.[id] : undefined
  }
  return { ego: at(0), at }
}

/** The blood term for what B (of sex g) is to the ego, ignoring half-blood and adoption marks. */
function bloodTerm(k: KinshipPart, g: Gender, c: Context): Term {
  const { up, down } = k
  const egoG = gender(c.ego?.sex)
  if (down === 0) {
    if (up === 1) return g === 'M' ? one('әке') : g === 'F' ? one('шеше') : one('ата-ана')
    if (up === 2) {
      // Through the mother (the mother's parents) the grandparents are нағашы.
      const maternal = c.at(1)?.sex === 'F'
      const term = bySex(g, () => one('ата'), () => one('әже'))
      return maternal ? withPrefix(term, 'нағашы') : term
    }
    if (up === 3) return bySex(g, () => one('арғы ата'), () => one('арғы әже'))
    return [phrase(`${ordinal(up)} буын ата-бабасы`)]
  }
  if (up === 0) {
    if (down === 1) return g === 'M' ? one('ұл') : g === 'F' ? one('қыз') : one('бала')
    // Children of a daughter are жиен, of a son немере.
    if (down === 2) return c.at(1)?.sex === 'F' ? one('жиен') : one('немере')
    if (down === 3) return one('шөбере')
    if (down === 4) return one('шөпшек')
    if (down === 5) return one('немене')
    return [phrase(`${ordinal(down)} буын ұрпағы`)]
  }
  if (up === 1 && down === 1) return sibling(g, rankOf(c.at(k.path ? k.path.length - 1 : -1), c.ego), egoG)
  if (down === 1 && up === 2) return parentSibling(g, c)
  if (down === 1 && up === 3) return bySex(g, () => one('ата'), () => one('әже'))
  if (up === 1 && down === 2) return childOfSibling(g, c)
  if (up === 2 && down === 2) {
    return firstCousin(g, rankOf(c.at(k.path ? k.path.length - 1 : -1), c.ego), egoG, c)
  }
  return [simple(`${ordinal(up + down)} буындағы туыс`)]
}

/** First cousin; path is A, parent, grandparent, B's parent, B. */
function firstCousin(g: Gender, rank: Rank, ego: Gender, c: Context): Term {
  const a = c.at(1)?.sex
  const b = c.at(3)?.sex
  const sib = sibling(g, rank, ego)
  if (a === 'F' && b === 'F') return one('бөле')
  if (a === 'F' && b === 'M') return sib.map((x) => simple(`нағашы ${x.nom}`))
  if (a === 'M' && b === 'F') return one('жиен')
  return sib.map((x) => simple(`немере ${x.nom}`))
}

/** Brother or sister of a parent; path is A, parent, grandparent, B. */
function parentSibling(g: Gender, c: Context): Term {
  const parent = c.at(1)
  const side = parent?.sex
  const rank = rankOf(c.at(3), parent)
  if (side === 'M') {
    return bySex(
      g,
      () => (rank === 'elder' ? one('көке') : [phrase(`әкесінің ${rank === 'younger' ? 'інісі' : 'бауыры'}`)]),
      () => one('тәте'),
    )
  }
  if (side === 'F') return bySex(g, () => one('нағашы аға'), () => one('нағашы апа'))
  // The parent is unknown or of unknown sex: say it through "parent's".
  const name = (m: boolean) =>
    rank === 'elder' ? (m ? 'ағасы' : 'әпкесі') : rank === 'younger' ? (m ? 'інісі' : 'қарындасы немесе сіңлісі') : 'бауыры'
  return bySex(
    g,
    () => [phrase(`ата-анасының ${name(true)}`)],
    () => [phrase(`ата-анасының ${name(false)}`)],
  )
}

/** Child of a brother or sister; path is A, parent, sibling, B. */
function childOfSibling(g: Gender, c: Context): Term {
  const s = c.at(2)
  // The children of a sister are жиен, whoever asks.
  if (s?.sex === 'F') return one('жиен')
  const owner = s?.sex === 'M' ? (rankOf(s, c.ego) === 'elder' ? 'ағасының' : rankOf(s, c.ego) === 'younger' ? 'інісінің' : 'бауырының') : 'бауырының'
  const child = g === 'M' ? 'ұлы' : g === 'F' ? 'қызы' : 'баласы'
  return [phrase(`${owner} ${child}`)]
}

/** Half-blood and adoptive marks on a term. */
function modify(term: Term, k: KinshipPart, halfSex: Sex | undefined): Term {
  const lineal = (k.up === 1 && k.down === 0) || (k.up === 0 && k.down === 1)
  const isSibling = k.up === 1 && k.down === 1
  if (k.adoptive) {
    if (k.up === 1 && k.down === 0) return withPrefix(term, 'асырап алған')
    return lineal || isSibling ? withPrefix(term, 'асыранды') : withTail(term, ' (асырап алу арқылы)')
  }
  if (k.half) {
    // The shared parent: the same father or the same mother.
    return withTail(term, halfSex === 'M' ? ' (әкесі бір)' : halfSex === 'F' ? ' (шешесі бір)' : ' (жартылай)')
  }
  return term
}

function kinship(k: KinshipPart, sex: Sex | undefined, persons: Persons | undefined): Term {
  const halfSex = k.half && k.ancestors?.[0] ? persons?.[k.ancestors[0]]?.sex : undefined
  return modify(bloodTerm(k, gender(sex), context(k, persons)), k, halfSex)
}

/** What someone of sex `sex` is to the other person, by a blood kinship. */
export function kinshipName(t: TFunction, k: KinshipPart, sex: Sex | undefined, persons?: Persons): string {
  return render(t, kinship(k, sex, persons))
}

const spouseTerm = (t: TFunction, sex: Sex | undefined) => t(`relationship.spouse_${gender(sex)}` as 'relationship.spouse_U')

/** The spouse as a possessed noun: "әйелі" (wife), "күйеуі" (husband), "жұбайы". */
const spouseHead = { M: 'күйеуі', F: 'әйелі', U: 'жұбайы' } as const

const is = (k: KinshipPart, up: number, down: number) => k.up === up && k.down === down

/** A word for a person of sex g, or "F or M" when unknown. */
function pick(t: TFunction, g: Gender, m: string, f: string): string {
  return g === 'M' ? m : g === 'F' ? f : t('relationship.either', { a: f, b: m })
}

/**
 * What B is to A, e.g. "жеңге" or "қайын ата"; null when they are not related.
 * Where the graph's link goes through a spouse, via is that spouse (or the
 * relative whose spouse B is).
 */
export function relationshipName(t: TFunction, rep: RelationshipReport): string | null {
  const b = rep.persons[rep.b]
  const via = rep.via ? rep.persons[rep.via] : undefined
  const k = rep.kinship
  const g = gender(b?.sex)
  const viaG = gender(via?.sex)
  const ref = (id: number | undefined) => (id !== undefined && id > 0 ? rep.persons[id] : undefined)
  switch (rep.kind) {
    case 'self':
      return t('relationship.self')
    case 'spouse':
      return spouseTerm(t, b?.sex)
    case 'blood':
      return k ? kinshipName(t, k, b?.sex, rep.persons) : null
    case 'spouse_of_relative': {
      if (!k) return null
      // The path runs from A to the relative and ends with B.
      const blood = { ...k, path: k.path.slice(0, -1) }
      const ego = ref(k.path[0])
      const rank = rankOf(via, ego)
      if (is(k, 1, 0)) return pick(t, g, 'өгей әке', 'өгей шеше')
      if (is(k, 0, 1)) return pick(t, g, 'күйеу бала', 'келін')
      if (is(k, 1, 1) && !k.half && g !== 'U' && viaG !== 'U') {
        // Sister's husband; brother's wife. Between two women or two men there is no term.
        if (g === 'M' && viaG === 'F') return rank === 'younger' ? 'күйеу бала' : 'жезде'
        if (g === 'F' && viaG === 'M') return rank === 'younger' ? 'келін' : 'жеңге'
      }
      return chain(t, kinship(blood, via?.sex, rep.persons), spouseHead[g])
    }
    case 'relative_of_spouse': {
      if (!k) return null
      // The path runs from A through the spouse to B.
      const blood = { ...k, path: k.path.slice(1) }
      const ego = ref(k.path[0])
      if (is(k, 1, 0)) return pick(t, g, 'қайын ата', 'қайын ене')
      if (is(k, 0, 1)) return g === 'U' ? 'өгей бала' : pick(t, g, 'өгей ұл', 'өгей қыз')
      if (is(k, 1, 1) && !k.half) {
        const rank = rankOf(b, via)
        // A wife's younger sister, as a man says it.
        if (rank === 'younger' && viaG === 'F' && gender(ego?.sex) === 'M') return 'балдыз'
        if (rank === undefined) return 'қайын бауыр'
        const elder = rank === 'elder'
        return pick(t, g, elder ? 'қайын аға' : 'қайын іні', elder ? 'қайын апа' : 'қайын сіңлі')
      }
      const head = kinship(blood, b?.sex, rep.persons)
      return `${genitive(spouseHead[viaG])} ${renderPossessed(t, head)}`
    }
    case 'spouse_blood_spouse': {
      if (!k) return null
      const via2 = rep.via2 ? rep.persons[rep.via2] : undefined
      if (is(k, 1, 1) && !k.half) {
        // Husbands of sisters: бажа; wives of brothers: абысын.
        if (g === 'M' && viaG === 'F' && via2?.sex === 'F') return 'бажа'
        if (g === 'F' && viaG === 'M' && via2?.sex === 'M') return 'абысын'
      }
      // The path runs A, the spouse, ..., the spouse's relative, B: "әйелінің ағасының әйелі".
      const relative = kinship({ ...k, path: k.path.slice(1, -1) }, via2?.sex, rep.persons)
      const owner = `${genitive(spouseHead[viaG])} ${renderPossessed(t, relative)}`
      const words = owner.split(' ')
      words[words.length - 1] = genitive(words[words.length - 1] ?? '')
      return `${words.join(' ')} ${spouseHead[g]}`
    }
    case 'step_sibling':
      return 'өгей бауыр'
    case 'co_parent_in_law':
      return pick(t, g, 'құда', 'құдағи')
    default:
      return null
  }
}
