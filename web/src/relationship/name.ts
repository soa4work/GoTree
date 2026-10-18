import i18n from 'i18next'
import type { TFunction } from 'i18next'
import type { Kinship, PersonRef, RelationshipReport, Sex } from '../api/types'
import * as en from './name.en'
import * as kk from './name.kk'
import * as ru from './name.ru'

type KinshipPart = Pick<Kinship, 'up' | 'down' | 'half' | 'adoptive'> & { ancestors?: number[]; path?: number[] }

/** The relationship terms of one language. */
interface Namer {
  kinshipName(t: TFunction, k: KinshipPart, sex: Sex | undefined, persons?: Record<number, PersonRef>): string
  relationshipName(t: TFunction, rep: RelationshipReport): string | null
}

// Languages have their own grammar (gender, case, seniority), so each builds
// its names itself; anything without a module uses the English pieces.
const namers: Record<string, Namer> = { ru, kk }

const namer = (): Namer => namers[i18n.language?.slice(0, 2)] ?? en

/**
 * What someone of sex `sex` is to the other person, by a blood kinship.
 * `persons` lets a language look at the people on the way (the shared parent
 * of half-siblings).
 */
export function kinshipName(t: TFunction, k: KinshipPart, sex: Sex | undefined, persons?: Record<number, PersonRef>): string {
  return namer().kinshipName(t, k, sex, persons)
}

/** What B is to A, e.g. "first cousin once removed" or "sister-in-law"; null when they are not related. */
export function relationshipName(t: TFunction, rep: RelationshipReport): string | null {
  return namer().relationshipName(t, rep)
}
