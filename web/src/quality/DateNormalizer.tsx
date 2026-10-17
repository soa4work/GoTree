import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getDateProposals, normalizeDates } from '../api/endpoints'
import type { DateProposal, DateProposals } from '../api/types'
import { Button } from '../components/Button'
import { fullName } from '../lib/people'
import { formatDate } from '../lib/dates'

export function DateNormalizer() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const proposals = useQuery({ queryKey: ['dates', 'proposals'], queryFn: ({ signal }) => getDateProposals(signal) })
  // Rows are opted out rather than in, so a fresh list starts all selected.
  const [excluded, setExcluded] = useState<Set<number>>(new Set())
  const apply = useMutation({
    mutationFn: (items: DateProposal[]) =>
      normalizeDates(items.map((p) => ({ eventId: p.eventId, raw: p.raw, date: p.proposed }))),
    onSuccess: () => {
      setExcluded(new Set())
      void queryClient.invalidateQueries({ queryKey: ['dates'] })
      void queryClient.invalidateQueries({ queryKey: ['checks'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
    },
  })

  if (proposals.isPending) return <p className="text-slate-600 dark:text-slate-400">{t('app.loading')}</p>
  if (proposals.isError) return <p role="alert">{t('common.loadFailed')}</p>

  const { items, unreadable } = proposals.data
  const selected = items.filter((p) => !excluded.has(p.eventId))
  const toggle = (id: number) =>
    setExcluded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  return (
    <div className="space-y-4">
      <p className="text-slate-700 dark:text-slate-300">{t('quality.datesIntro')}</p>
      <div role="status">
        {apply.isSuccess && (
          <p className="font-medium text-green-800 dark:text-green-300">
            {t('quality.applied', { count: apply.data.updated })}
            {apply.data.skipped > 0 && ` ${t('quality.skipped', { count: apply.data.skipped })}`}
          </p>
        )}
      </div>
      {apply.isError && (
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {t('common.saveFailed', { message: apply.error.message })}
        </p>
      )}
      {items.length === 0 ? (
        <p>{t('quality.datesNone')}</p>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="secondary" onClick={() => setExcluded(new Set())}>
              {t('quality.selectAll')}
            </Button>
            <Button variant="secondary" onClick={() => setExcluded(new Set(items.map((p) => p.eventId)))}>
              {t('quality.selectNone')}
            </Button>
            <span className="text-sm text-slate-600 dark:text-slate-400">
              {t('quality.selectedCount', { count: selected.length, total: items.length })}
            </span>
          </div>
          <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
            {items.map((p) => (
              <li key={p.eventId}>
                <ProposalRow proposal={p} persons={proposals.data.persons} checked={!excluded.has(p.eventId)} onToggle={() => toggle(p.eventId)} />
              </li>
            ))}
          </ul>
          <Button onClick={() => apply.mutate(selected)} disabled={selected.length === 0} busy={apply.isPending}>
            {t('quality.apply', { count: selected.length })}
          </Button>
        </>
      )}
      {unreadable > 0 && <p className="text-sm text-slate-600 dark:text-slate-400">{t('quality.datesUnreadable', { count: unreadable })}</p>}
    </div>
  )
}

function ProposalRow(props: {
  proposal: DateProposal
  persons: DateProposals['persons']
  checked: boolean
  onToggle: () => void
}) {
  const { proposal: p, persons, checked, onToggle } = props
  const { t } = useTranslation()
  const id = `date-${p.eventId}`
  const names = p.personIds.map((pid) => ({ id: pid, name: fullName(persons[pid] ?? { givenNames: '', surname: '' }) ?? t('person.unknown') }))
  const event = p.customLabel || t(`eventType.${p.type}`, { defaultValue: p.type })
  return (
    <div className="flex items-start gap-3 px-4 py-3">
      <input id={id} type="checkbox" checked={checked} onChange={onToggle} className="mt-1 size-5 shrink-0" />
      <div className="min-w-0">
        <label htmlFor={id} className="block">
          <span className="sr-only">{t('quality.dateRow', { raw: p.raw, proposed: p.proposed })}</span>
          <span aria-hidden="true">
            <span className="font-mono">{p.raw}</span> → <span className="font-semibold">{formatDate(p.proposed)}</span>
          </span>
          <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">({t('quality.dateRepair')})</span>
        </label>
        <p className="text-sm text-slate-600 dark:text-slate-400">
          {event} ·{' '}
          {p.familyId !== null ? (
            t('quality.family', { names: names.map((n) => n.name).join(' & ') })
          ) : (
            names.map((n) => (
              <Link key={n.id} to={`/people/${n.id}`} className="inline-flex min-h-11 items-center underline">
                {n.name}
              </Link>
            ))
          )}
        </p>
      </div>
    </div>
  )
}
