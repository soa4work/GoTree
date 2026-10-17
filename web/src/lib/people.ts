import type { PersonInput, PersonRef, PlaceRef } from '../api/types'

type Named = Pick<PersonRef, 'givenNames' | 'surname'>

/** The name to show, or null when nothing is known (callers show "Unknown"). */
export function fullName(p: Named): string | null {
  const name = [p.givenNames, p.surname].filter(Boolean).join(' ')
  return name || null
}

/** "1850 – 1912", "* 1850", "† 1912" or "" from GEDCOM dates. */
export function lifespan(p: Pick<PersonRef, 'birthDate' | 'deathDate'>): string {
  const birth = yearOf(p.birthDate)
  const death = yearOf(p.deathDate)
  if (birth && death) return `${birth} – ${death}`
  if (birth) return `* ${birth}`
  if (death) return `† ${death}`
  return ''
}

/** The year with its qualifier ("ABT 1850" → "~1850"), or the raw text. */
export function yearOf(date: string): string {
  if (!date) return ''
  if (/^(BET|FROM)\b/.test(date)) {
    const first = date.match(/\b(\d{3,4})\b/)?.[1]
    return first ? `~${first}` : date
  }
  const year = date.match(/(\d{3,4})(?:\/\d{2})?(?:\s+B\.C\.)?\s*$/)?.[1] ?? date.match(/\b(\d{3,4})\b/)?.[1]
  if (!year) return date
  if (/^(ABT|CAL|EST)\b/.test(date)) return `~${year}`
  if (/^BEF\b/.test(date)) return `<${year}`
  if (/^AFT\b/.test(date)) return `>${year}`
  return year
}

export const emptyPerson = (): PersonInput => ({
  givenNames: '',
  surname: '',
  namePrefix: '',
  nameSuffix: '',
  nickname: '',
  sex: 'U',
  isLiving: null,
  notes: '',
  alternateNames: [],
})

/** What is typed for a birth or death in the create forms. */
export interface LifeDraft {
  date: string
  place: PlaceRef | null
}

export interface LifeDrafts {
  birth: LifeDraft
  death: LifeDraft
}

export const emptyLife = (): LifeDrafts => ({ birth: { date: '', place: null }, death: { date: '', place: null } })

/** The birth and death to send; empty ones are left out, and no death for someone living. */
export function lifeInput(life: LifeDrafts, isLiving: boolean | null): Pick<PersonInput, 'birth' | 'death'> {
  const of = (d: LifeDraft) => (d.date.trim() || d.place ? { date: d.date.trim(), placeId: d.place?.id ?? null } : undefined)
  const birth = of(life.birth)
  const death = isLiving === true ? undefined : of(life.death)
  return { ...(birth ? { birth } : {}), ...(death ? { death } : {}) }
}
