import { useQuery } from '@tanstack/react-query'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import type { DayReport } from '../api/types'
import { fullName } from '../lib/people'
import { usePersonHref } from '../lib/personHref'

interface Props {
  queryKey: readonly unknown[]
  load: (month: number, day: number, signal: AbortSignal) => Promise<DayReport>
  /** Render nothing unless there are entries to show. */
  hideEmpty?: boolean
}

/** Births, marriages and deaths of deceased relatives on today's date. */
export function OnThisDay({ queryKey, load, hideEmpty }: Props) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const href = usePersonHref()
  // The visitor's own calendar day, not the server's.
  const today = new Date()
  const month = today.getMonth() + 1
  const day = today.getDate()
  const report = useQuery({
    queryKey: [...queryKey, 'onthisday', month, day],
    queryFn: ({ signal }) => load(month, day, signal),
  })
  const dateText = today.toLocaleDateString(i18n.language, { day: 'numeric', month: 'long' })

  if (hideEmpty && (!report.data || report.data.events.length === 0)) return null

  return (
    <section aria-labelledby={id} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
      <h2 id={id} className="text-xl font-semibold">
        {t('onThisDay.title', { date: dateText })}
      </h2>
      {report.isPending && <p className="mt-2 text-slate-600 dark:text-slate-400">{t('app.loading')}</p>}
      {report.isError && <p className="mt-2">{t('common.loadFailed')}</p>}
      {report.data && report.data.events.length === 0 && (
        <p className="mt-2 text-slate-600 dark:text-slate-400">{t('onThisDay.none')}</p>
      )}
      {report.data && report.data.events.length > 0 && (
        <ul className="mt-2">
          {report.data.events.map((e) => {
            const people = e.personIds.map((pid) => ({
              id: pid,
              name: fullName(report.data.persons[pid] ?? { givenNames: '', surname: '' }) ?? t('person.unknown'),
            }))
            return (
              <li key={e.eventId} className="flex items-baseline gap-3">
                <span className="w-12 shrink-0 font-semibold tabular-nums">{e.year}</span>
                <span>
                  {people.map((p, i) => (
                    <span key={p.id}>
                      {i > 0 && ` ${t('onThisDay.and')} `}
                      <Link
                        to={href(p.id)}
                        className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100"
                      >
                        {p.name}
                      </Link>
                    </span>
                  ))}{' '}
                  {t(`onThisDay.${e.type}`, { count: people.length })}
                  <span className="text-sm text-slate-600 dark:text-slate-400">
                    {' '}
                    · {t('onThisDay.yearsAgo', { count: today.getFullYear() - e.year })}
                  </span>
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
