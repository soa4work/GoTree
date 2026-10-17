import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getAuthState, getHealth } from '../api/client'
import { getOnThisDay, getStats, listSources, listTasks } from '../api/endpoints'
import { OnboardingCard } from '../components/OnboardingCard'
import { PageHeading } from '../components/PageHeading'
import { TaskList } from '../research/ResearchPanel'
import { OnThisDay } from '../share/OnThisDay'

const linkClass = 'inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100'

export function Dashboard() {
  const { t } = useTranslation()
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  const isAdmin = auth.data?.user?.role === 'admin'
  const health = useQuery({
    queryKey: ['health'],
    queryFn: ({ signal }) => getHealth(signal),
    enabled: isAdmin,
  })
  const stats = useQuery({ queryKey: ['stats'], queryFn: ({ signal }) => getStats(signal) })
  const sources = useQuery({ queryKey: ['sources', 'dashboard'], queryFn: ({ signal }) => listSources('', signal) })
  const tasks = useQuery({
    queryKey: ['research', 'tasks', 'dashboard'],
    queryFn: ({ signal }) => listTasks({ status: 'active', limit: 5 }, signal),
  })

  const empty = stats.data?.persons === 0
  const filled = stats.data !== undefined && !empty

  let status: string
  if (health.isPending) status = t('dashboard.checking')
  else if (health.isError) status = t('dashboard.offline')
  else status = t('dashboard.online', { version: health.data.version })

  return (
    <div className="max-w-3xl space-y-6">
      <PageHeading title={t('app.name')}>{t('dashboard.title')}</PageHeading>
      <p className="text-slate-700 dark:text-slate-300">{t('dashboard.intro')}</p>
      {empty && <OnboardingCard />}
      {filled && (
        <section aria-labelledby="tree-summary" className="space-y-2">
          <h2 id="tree-summary" className="text-xl font-semibold">
            {t('dashboard.summary')}
          </h2>
          <dl className="grid gap-3 sm:grid-cols-3">
            {(
              [
                ['dashboard.people', '/people', stats.data.persons],
                ['dashboard.families', null, stats.data.families],
                ['dashboard.sources', '/sources', sources.data?.length],
              ] as const
            ).map(([key, to, count]) => (
              <div key={key} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
                <dt className="text-sm font-semibold text-slate-600 dark:text-slate-400">
                  {to ? (
                    <Link to={to} className="underline">
                      {t(key)}
                    </Link>
                  ) : (
                    t(key)
                  )}
                </dt>
                <dd className="mt-1 text-2xl font-semibold">{count ?? '–'}</dd>
              </div>
            ))}
          </dl>
        </section>
      )}
      {filled && <OnThisDay queryKey={['dashboard']} load={getOnThisDay} hideEmpty />}
      {tasks.data && tasks.data.length > 0 && (
        <section aria-labelledby="open-tasks" className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 id="open-tasks" className="text-xl font-semibold">
              {t('research.openTasks')}
            </h2>
            <Link to="/research" className={linkClass}>
              {t('research.allTasks')}
            </Link>
          </div>
          <TaskList tasks={tasks.data} pending={false} />
        </section>
      )}
      {/* The wall chart is in no navigation; phones also lack room for the rest of the sidebar. */}
      <nav aria-labelledby="dashboard-also">
        <h2 id="dashboard-also" className="text-sm font-semibold text-slate-600 dark:text-slate-400">
          {t('dashboard.also')}
        </h2>
        <ul className="flex flex-wrap gap-x-5">
          {(
            [
              ['/chart', 'nav.chart', ''],
              ['/media', 'nav.media', 'md:hidden'],
              ['/heirlooms', 'nav.heirlooms', 'md:hidden'],
              ['/map', 'nav.map', 'md:hidden'],
              ['/stats', 'nav.stats', 'md:hidden'],
              ['/transcribe', 'nav.transcribe', 'md:hidden'],
              ['/research', 'nav.research', 'md:hidden'],
              ['/relationship', 'nav.relationship', 'md:hidden'],
              ['/quality', 'nav.quality', 'md:hidden'],
              ['/sharing', 'nav.sharing', 'md:hidden'],
            ] as const
          ).map(([to, key, cls]) => (
            <li key={to} className={cls}>
              <Link to={to} className={linkClass}>
                {t(key)}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
      {isAdmin && (
        <p role="status" className="flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
          <span className="sr-only">{t('dashboard.serverStatus')}: </span>
          <span
            aria-hidden="true"
            className={[
              'inline-block size-2.5 rounded-full',
              health.isPending ? 'bg-slate-400' : health.isError ? 'bg-red-600' : 'bg-leaf-500',
            ].join(' ')}
          />
          {status}
        </p>
      )}
    </div>
  )
}
