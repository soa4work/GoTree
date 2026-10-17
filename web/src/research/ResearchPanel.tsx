import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { createTask, getSuggestions, listLog, listTasks, type ResearchQuery } from '../api/endpoints'
import type { LogEntry, ResearchLink, ResearchTask, Suggestion } from '../api/types'
import { Button } from '../components/Button'
import { SelectField } from '../components/Field'
import { LogDialog } from './LogDialog'
import { TaskDialog } from './TaskDialog'
import { formatDay } from '../lib/dates'

type Open = { kind: 'task'; task?: ResearchTask } | { kind: 'log'; entry?: LogEntry; taskId?: number | null } | null

const day = formatDay

function todayISO() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

interface Props {
  /** Limits the panel to one person and links new items to them. */
  person?: { id: number; label: string }
  headingLevel: 2 | 3
}

/** Tasks and log of the tree, or of one person. */
export function ResearchPanel({ person, headingLevel }: Props) {
  const { t } = useTranslation()
  const [status, setStatus] = useState(person ? '' : 'active')
  const [open, setOpen] = useState<Open>(null)
  const filter: ResearchQuery = { status, person: person?.id }
  const tasks = useQuery({
    queryKey: ['research', 'tasks', filter],
    queryFn: ({ signal }) => listTasks(filter, signal),
  })
  const log = useQuery({
    queryKey: ['research', 'log', person?.id ?? null],
    queryFn: ({ signal }) => listLog({ person: person?.id, limit: 100 }, signal),
  })
  const tasksId = useId()
  const logId = useId()
  const H = `h${headingLevel}` as const
  const initialLinks: ResearchLink[] = person
    ? [{ entityType: 'person', entityId: person.id, label: person.label }]
    : []

  return (
    <div className="space-y-6">
      {person && <Suggestions personId={person.id} label={person.label} />}

      <section aria-labelledby={tasksId} className="space-y-3">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <H id={tasksId} className="text-lg font-semibold">
            {t('research.tasks')}
          </H>
          <div className="flex flex-wrap items-end gap-2">
            {!person && (
              <SelectField label={t('research.filter')} value={status} onChange={(e) => setStatus(e.target.value)}>
                <option value="active">{t('research.filterActive')}</option>
                <option value="done">{t('research.status_done')}</option>
                <option value="">{t('research.filterAll')}</option>
              </SelectField>
            )}
            <Button variant="secondary" onClick={() => setOpen({ kind: 'task' })}>
              + {t('research.newTask')}
            </Button>
          </div>
        </div>
        <TaskList tasks={tasks.data} pending={tasks.isPending} onOpen={(task) => setOpen({ kind: 'task', task })} />
      </section>

      <section aria-labelledby={logId} className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <H id={logId} className="text-lg font-semibold">
            {t('research.log')}
          </H>
          <Button variant="secondary" onClick={() => setOpen({ kind: 'log' })}>
            + {t('research.newEntry')}
          </Button>
        </div>
        <LogList entries={log.data} pending={log.isPending} onOpen={(entry) => setOpen({ kind: 'log', entry })} />
      </section>

      {open?.kind === 'task' && (
        <TaskDialog task={open.task} initialLinks={initialLinks} onClose={() => setOpen(null)} />
      )}
      {open?.kind === 'log' && (
        <LogDialog
          entry={open.entry}
          initialLinks={initialLinks}
          initialTaskId={open.taskId ?? null}
          onClose={() => setOpen(null)}
        />
      )}
    </div>
  )
}

export function TaskList({
  tasks,
  pending,
  onOpen,
}: {
  tasks: ResearchTask[] | undefined
  pending: boolean
  onOpen?: (task: ResearchTask) => void
}) {
  const { t } = useTranslation()
  if (pending) return <p className="text-slate-600 dark:text-slate-400">{t('app.loading')}</p>
  if (!tasks || tasks.length === 0) return <p className="text-slate-600 dark:text-slate-400">{t('research.noTasks')}</p>
  const today = todayISO()
  return (
    <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
      {tasks.map((task) => {
        const overdue = task.status !== 'done' && task.dueOn !== '' && task.dueOn < today
        const meta = [
          t(`research.status_${task.status}`),
          task.priority !== 'normal' ? t(`research.priority_${task.priority}`) : '',
          task.dueOn ? t(overdue ? 'research.overdue' : 'research.due', { date: day(task.dueOn) }) : '',
          task.logCount > 0 ? t('research.logCount', { count: task.logCount }) : '',
          task.links.map((l) => l.label).join(', '),
        ].filter(Boolean)
        const body = (
          <>
            <span
              className={[
                'block font-medium',
                task.status === 'done' ? 'text-slate-600 line-through dark:text-slate-400' : '',
              ].join(' ')}
            >
              {task.title}
            </span>
            <span
              className={[
                'block text-sm',
                overdue ? 'font-medium text-red-700 dark:text-red-400' : 'text-slate-600 dark:text-slate-400',
              ].join(' ')}
            >
              {meta.join(' · ')}
            </span>
          </>
        )
        return (
          <li key={task.id}>
            {onOpen ? (
              <button
                type="button"
                onClick={() => onOpen(task)}
                className="block min-h-11 w-full px-4 py-2 text-left hover:bg-slate-50 dark:hover:bg-slate-900"
              >
                {body}
              </button>
            ) : (
              <Link to="/research" className="block min-h-11 px-4 py-2 hover:bg-slate-50 dark:hover:bg-slate-900">
                {body}
              </Link>
            )}
          </li>
        )
      })}
    </ul>
  )
}

function LogList({
  entries,
  pending,
  onOpen,
}: {
  entries: LogEntry[] | undefined
  pending: boolean
  onOpen: (e: LogEntry) => void
}) {
  const { t } = useTranslation()
  if (pending) return <p className="text-slate-600 dark:text-slate-400">{t('app.loading')}</p>
  if (!entries || entries.length === 0)
    return <p className="text-slate-600 dark:text-slate-400">{t('research.noEntries')}</p>
  return (
    <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
      {entries.map((e) => (
        <li key={e.id}>
          <button
            type="button"
            onClick={() => onOpen(e)}
            className="flex min-h-11 w-full gap-3 px-4 py-2 text-left hover:bg-slate-50 dark:hover:bg-slate-900"
          >
            <span
              className={[
                'mt-0.5 h-fit shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold',
                e.result === 'found'
                  ? 'bg-green-100 text-green-900 dark:bg-green-950 dark:text-green-200'
                  : e.result === 'partial'
                    ? 'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200'
                    : 'bg-slate-200 text-slate-800 dark:bg-slate-800 dark:text-slate-200',
              ].join(' ')}
            >
              {t(`research.result_${e.result}`)}
            </span>
            <span className="min-w-0">
              <span className="block font-medium">{e.query}</span>
              <span className="block text-sm text-slate-600 dark:text-slate-400">
                {[
                  day(e.searchedOn),
                  e.location,
                  e.taskTitle ? t('research.forTask', { title: e.taskTitle }) : '',
                  e.links.map((l) => l.label).join(', '),
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function Suggestions({ personId, label }: { personId: number; label: string }) {
  const { t } = useTranslation()
  const id = useId()
  const queryClient = useQueryClient()
  const sugg = useQuery({
    queryKey: ['person', personId, 'suggestions'],
    queryFn: ({ signal }) => getSuggestions(personId, signal),
  })
  // Findings have their own button in the list of possible problems.
  const items = (sugg.data?.suggestions ?? []).filter((s) => s.kind !== 'finding')
  const title = (s: Suggestion) => t(`research.suggestion_${s.kind as 'missing_birth'}`, { name: label })
  const make = useMutation({
    mutationFn: (s: Suggestion) =>
      createTask({
        title: title(s),
        status: 'open',
        priority: 'normal',
        dueOn: '',
        notes: '',
        origin: s.origin,
        links: [{ entityType: 'person', entityId: personId }],
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['person', personId] })
      void queryClient.invalidateQueries({ queryKey: ['research'] })
    },
  })
  if (items.length === 0) return null
  return (
    <section aria-labelledby={id} className="space-y-2">
      <h3 id={id} className="text-lg font-semibold">
        {t('research.suggestions')}
      </h3>
      <ul className="space-y-2">
        {items.map((s) => (
          <li
            key={s.origin}
            className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-dashed border-slate-300 px-4 py-2 dark:border-slate-700"
          >
            <span>{title(s)}</span>
            <Button
              variant="ghost"
              busy={make.isPending && make.variables?.origin === s.origin}
              onClick={() => make.mutate(s)}
            >
              {t('research.makeTask')}
              <span className="sr-only">: {title(s)}</span>
            </Button>
          </li>
        ))}
      </ul>
    </section>
  )
}

