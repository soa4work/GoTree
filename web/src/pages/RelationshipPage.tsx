import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'
import { getPerson, getRelationship } from '../api/endpoints'
import type { PersonDetail, PersonRef, RelationshipReport } from '../api/types'
import { Button } from '../components/Button'
import { PageHeading } from '../components/PageHeading'
import { fullName, lifespan } from '../lib/people'
import { PersonPicker } from '../people/PersonPicker'
import { kinshipName, relationshipName } from '../relationship/name'
import { relationshipPath } from '../relationship/path'

type Pair = { a: PersonRef | null; b: PersonRef | null }

/**
 * The two people, kept in the URL (?a=12&b=34) so links can preselect
 * them. Both change through one URL update, so swapping cannot lose one.
 */
function usePeopleParams(): [Pair, (next: Pair) => void] {
  const [params, setParams] = useSearchParams()
  const [picked, setPicked] = useState<Record<number, PersonRef>>({})
  const idA = Number(params.get('a')) || 0
  const idB = Number(params.get('b')) || 0
  const load = (id: number) => ({
    queryKey: ['person', id],
    queryFn: ({ signal }: { signal: AbortSignal }) => getPerson(id, signal),
    enabled: id > 0 && !picked[id],
  })
  const loadedA = useQuery(load(idA))
  const loadedB = useQuery(load(idB))
  const resolve = (id: number, loaded: PersonDetail | undefined): PersonRef | null =>
    id === 0 ? null : (picked[id] ?? (loaded ? { ...loaded, birthDate: '', deathDate: '' } : null))
  const set = (next: Pair) => {
    setPicked((prev) => ({
      ...prev,
      ...(next.a ? { [next.a.id]: next.a } : {}),
      ...(next.b ? { [next.b.id]: next.b } : {}),
    }))
    const q = new URLSearchParams()
    if (next.a) q.set('a', String(next.a.id))
    if (next.b) q.set('b', String(next.b.id))
    setParams(q, { replace: true })
  }
  return [{ a: resolve(idA, loadedA.data), b: resolve(idB, loadedB.data) }, set]
}

export function RelationshipPage() {
  const { t } = useTranslation()
  const [{ a, b }, setPair] = usePeopleParams()
  const rel = useQuery({
    queryKey: ['relationship', a?.id, b?.id],
    queryFn: ({ signal }) => getRelationship(a!.id, b!.id, signal),
    enabled: a !== null && b !== null,
  })

  return (
    <div className="max-w-3xl space-y-6">
      <div className="space-y-3">
        <PageHeading title={t('nav.relationship')}>{t('nav.relationship')}</PageHeading>
        <p className="text-slate-700 dark:text-slate-300">{t('relationship.intro')}</p>
      </div>
      <div className="grid gap-4 sm:grid-cols-[1fr_auto_1fr] sm:items-end">
        <PersonPicker label={t('relationship.personA')} value={a} onChange={(p) => setPair({ a: p, b })} />
        <Button
          variant="secondary"
          disabled={!a || !b}
          onClick={() => setPair({ a: b, b: a })}
        >
          ⇄ {t('relationship.swap')}
        </Button>
        <PersonPicker label={t('relationship.personB')} value={b} onChange={(p) => setPair({ a, b: p })} />
      </div>
      <div role="status">
        {rel.isFetching && <p className="text-slate-600 dark:text-slate-400">{t('relationship.calculating')}</p>}
        {rel.isError && <p className="font-medium text-red-700 dark:text-red-400">{t('common.loadFailed')}</p>}
        {rel.data && !rel.isFetching && <Sentence rep={rel.data} />}
      </div>
      {rel.data && !rel.isFetching && <Details rep={rel.data} />}
    </div>
  )
}

function useNames(rep: RelationshipReport) {
  const { t } = useTranslation()
  return (id: number) =>
    id < 0 ? t('relationship.unknownParents') : (fullName(rep.persons[id] ?? { givenNames: '', surname: '' }) ?? t('person.unknown'))
}

function Sentence({ rep }: { rep: RelationshipReport }) {
  const { t } = useTranslation()
  const name = useNames(rep)
  const relation = relationshipName(t, rep)
  let text: string
  if (rep.kind === 'self') text = t('relationship.same')
  else if (relation === null) text = t('relationship.notRelated', { a: name(rep.a), b: name(rep.b) })
  else text = t('relationship.sentence', { a: name(rep.a), b: name(rep.b), relation })
  return <p className="rounded-xl bg-brand-50 p-4 text-lg font-medium dark:bg-slate-900">{text}</p>
}

function Details({ rep }: { rep: RelationshipReport }) {
  const { t } = useTranslation()
  const name = useNames(rep)
  const pathId = useId()
  const steps = relationshipPath(rep)
  const ancestors = (rep.kinship?.ancestors ?? []).filter((id) => id > 0)
  if (steps.length < 2) return null
  return (
    <div className="space-y-6">
      <section aria-labelledby={pathId} className="space-y-3">
        <h2 id={pathId} className="text-xl font-semibold">
          {t('relationship.path')}
        </h2>
        <ol className="space-y-2">
          {steps.map((s, i) => {
            const person = rep.persons[s.id]
            return (
              <li key={`${s.id}-${i}`} className="rounded-lg border border-slate-200 px-4 py-2 dark:border-slate-800">
                {s.link && (
                  <span className="block text-sm text-slate-600 dark:text-slate-400">
                    {t(`relationship.pathStep${s.link === 'parent' ? 'Parent' : s.link === 'child' ? 'Child' : 'Spouse'}`)}
                  </span>
                )}
                {s.id > 0 ? (
                  <Link to={`/people/${s.id}`} className="font-medium text-brand-700 underline dark:text-brand-100">
                    {name(s.id)}
                  </Link>
                ) : (
                  <span className="font-medium">{name(s.id)}</span>
                )}
                {person && lifespan(person) && <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">{lifespan(person)}</span>}
              </li>
            )
          })}
        </ol>
        {ancestors.length > 0 && rep.kind === 'blood' && (rep.kinship?.up ?? 0) > 0 && (rep.kinship?.down ?? 0) > 0 && (
          <p>{t('relationship.commonAncestors', { count: ancestors.length, names: ancestors.map(name).join(' & ') })}</p>
        )}
      </section>
      {rep.others.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-xl font-semibold">{t('relationship.others')}</h2>
          <ul className="list-disc space-y-1 pl-6">
            {rep.others.map((k) => (
              <li key={`${k.up}-${k.down}-${k.ancestors.join()}`}>
                {t('relationship.otherLine', {
                  relation: kinshipName(t, k, rep.persons[rep.b]?.sex),
                  names: k.ancestors.map(name).join(' & '),
                })}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
