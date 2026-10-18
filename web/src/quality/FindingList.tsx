import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { createTask } from '../api/endpoints'
import type { CheckReport, Finding } from '../api/types'
import { Button } from '../components/Button'
import { errorText } from '../lib/errors'
import { fullName } from '../lib/people'
import { useFindingText } from './findingText'

interface Props {
  report: CheckReport
  /** Leaves out the link to this person, e.g. on their own page. */
  currentPersonId?: number
}

export function FindingList({ report, currentPersonId }: Props) {
  const { t } = useTranslation()
  const text = useFindingText()
  const queryClient = useQueryClient()
  const makeTask = useMutation({
    mutationFn: (f: Finding) =>
      createTask({
        title: text(f, report.persons),
        status: 'open',
        priority: f.severity === 'error' ? 'high' : 'normal',
        dueOn: '',
        notes: '',
        origin: f.origin,
        links: [f.personId, f.otherPersonId]
          .filter((id): id is number => id !== undefined)
          .map((id) => ({ entityType: 'person' as const, entityId: id })),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['checks'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      void queryClient.invalidateQueries({ queryKey: ['research'] })
    },
  })
  return (
    <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
      {report.findings.map((f, i) => {
        const people = [f.personId, f.otherPersonId].filter(
          (id): id is number => id !== undefined && id !== currentPersonId,
        )
        return (
          <li key={`${f.rule}-${f.personId}-${f.eventId ?? f.otherPersonId ?? i}`} className="flex gap-3 px-4 py-3">
            <span
              className={[
                'mt-0.5 h-fit shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold',
                f.severity === 'error'
                  ? 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200'
                  : 'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200',
              ].join(' ')}
            >
              {t(`quality.severity_${f.severity}`)}
            </span>
            <div className="min-w-0 flex-1">
              <p>{text(f, report.persons)}</p>
              {people.length > 0 && (
                <p className="flex flex-wrap gap-x-4 text-sm">
                  {people.map((id) => (
                    <Link
                      key={id}
                      to={`/people/${id}`}
                      className="inline-flex min-h-11 items-center text-brand-700 underline dark:text-brand-100"
                    >
                      {fullName(report.persons[id] ?? { givenNames: '', surname: '' }) ?? t('person.unknown')}
                    </Link>
                  ))}
                </p>
              )}
              {makeTask.isError && makeTask.variables?.origin === f.origin && (
                <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
                  {t('common.saveFailed', { message: errorText(makeTask.error, t) })}
                </p>
              )}
            </div>
            {report.taskIds[f.origin] ? (
              <Link to="/research" className="inline-flex min-h-11 shrink-0 items-center text-sm text-slate-600 underline dark:text-slate-400">
                {t('research.taskExists')}
              </Link>
            ) : (
              <Button
                variant="ghost"
                className="shrink-0"
                busy={makeTask.isPending && makeTask.variables?.origin === f.origin}
                onClick={() => makeTask.mutate(f)}
              >
                {t('research.makeTask')}
                <span className="sr-only">: {text(f, report.persons)}</span>
              </Button>
            )}
          </li>
        )
      })}
    </ul>
  )
}
