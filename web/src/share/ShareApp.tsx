import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GopherCredit } from '../components/GopherCredit'
import { Link, NavLink, Route, Routes, useLocation, useParams, useSearchParams } from 'react-router'
import { ApiError } from '../api/client'
import { shareApi } from '../api/endpoints'
import type { Family, PersonRef, ShareInfo } from '../api/types'
import { SelectField } from '../components/Field'
import { LanguageSwitch } from '../components/LanguageSwitch'
import { PageHeading } from '../components/PageHeading'
import { ThemeSwitch } from '../components/ThemeSwitch'
import { useDebounced } from '../hooks/useDebounced'
import { fullName, lifespan } from '../lib/people'
import { PersonHrefContext, usePersonHref } from '../lib/personHref'
import { treeDisplayName } from '../lib/treeName'
import { Timeline } from '../people/Timeline'
import { timelineItems } from '../people/timelineItems'
import { layoutTree, type TreeView } from '../tree/layout'
import { indexGraph } from '../tree/model'
import { TreeCanvas } from '../tree/TreeCanvas'
import { TreeList } from '../tree/TreeList'
import { OnThisDay } from './OnThisDay'

/**
 * The read-only view behind a share link, for visitors without an
 * account. It talks only to /api/share/{token}, which applies the link's
 * scope and privacy on the server.
 */
export function ShareApp() {
  const { t } = useTranslation()
  const token = useParams().token ?? ''
  const api = useMemo(() => shareApi(token), [token])
  const info = useQuery({ queryKey: ['share', token, 'info'], queryFn: ({ signal }) => api.info(signal), retry: false })
  const base = `/share/${token}`
  const href = useMemo(() => (id: number) => `${base}/people/${id}`, [base])
  const location = useLocation()
  const mainRef = useRef<HTMLElement>(null)
  const firstRender = useRef(true)

  // Runs on purpose after each navigation.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    const heading = mainRef.current?.querySelector<HTMLElement>('h1')
    ;(heading ?? mainRef.current)?.focus()
  }, [location.pathname])

  if (info.isError) {
    const gone = info.error instanceof ApiError && info.error.status === 404
    return (
      <main id="main" className="mx-auto max-w-xl space-y-3 p-6">
        <PageHeading title={t('share.invalid')}>{gone ? t('share.invalid') : t('common.loadFailed')}</PageHeading>
        {gone && <p>{t('share.invalidHint')}</p>}
      </main>
    )
  }

  const tab = (to: string, label: string, end = false) => (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        [
          'flex min-h-11 items-center rounded-lg px-3 font-medium',
          isActive
            ? 'bg-brand-50 text-brand-700 dark:bg-slate-800 dark:text-brand-100'
            : 'text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-900',
        ].join(' ')
      }
    >
      {label}
    </NavLink>
  )

  return (
    <PersonHrefContext.Provider value={href}>
      <div className="min-h-dvh bg-slate-50 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-brand-700 focus:px-4 focus:py-3 focus:text-white"
        >
          {t('app.skipToContent')}
        </a>
        <header className="border-b border-slate-200 bg-slate-100 px-4 py-2 dark:border-slate-800 dark:bg-slate-950">
          <div className="flex flex-wrap items-center gap-3">
            <img src="/icons/logo-64.png" alt="" width={32} height={32} className="size-8 rounded-md" />
            <span className="min-w-0 truncate text-lg font-semibold text-brand-700 dark:text-brand-100">
              {info.data ? t('share.sharedBy', { tree: treeDisplayName(info.data.treeName, t) }) : t('app.name')}
            </span>
            <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700 dark:bg-slate-800 dark:text-slate-200">
              {t('share.readOnly')}
            </span>
            <div className="ml-auto flex items-center gap-2">
              <LanguageSwitch />
              <ThemeSwitch />
            </div>
          </div>
          <nav aria-label={t('app.mainNavigation')} className="mt-2">
            <ul className="flex gap-1">
              <li>{tab(base, t('share.home'), true)}</li>
              <li>{tab(`${base}/people`, t('share.people'))}</li>
              <li>{tab(`${base}/tree`, t('share.tree'))}</li>
            </ul>
          </nav>
        </header>
        <main id="main" ref={mainRef} tabIndex={-1} className="px-4 pt-6 pb-12 focus:outline-none md:px-8">
          {info.isPending ? (
            <p role="status">{t('app.loading')}</p>
          ) : (
            <Routes>
              <Route index element={<ShareHome info={info.data} token={token} />} />
              <Route path="people" element={<SharePeople token={token} />} />
              <Route path="people/:id" element={<SharePerson token={token} />} />
              <Route path="tree" element={<ShareTree token={token} startId={info.data.startId} />} />
            </Routes>
          )}
        </main>
        <footer className="border-t border-slate-200 px-4 py-4 md:px-8 dark:border-slate-800">
          <GopherCredit />
        </footer>
      </div>
    </PersonHrefContext.Provider>
  )
}

function PrivacyNote({ info }: { info: ShareInfo }) {
  const { t } = useTranslation()
  return (
    <p className="text-slate-700 dark:text-slate-300">
      {info.privacy === 'deceased' ? t('share.deceasedNote') : t('share.livingNamesNote')}
    </p>
  )
}

function ShareHome({ info, token }: { info: ShareInfo; token: string }) {
  const { t } = useTranslation()
  const href = usePersonHref()
  const api = shareApi(token)
  const startName = info.root ? fullName(info.root) : null
  return (
    <div className="max-w-3xl space-y-6">
      <PageHeading title={treeDisplayName(info.treeName, t)}>{treeDisplayName(info.treeName, t)}</PageHeading>
      <PrivacyNote info={info} />
      {info.startId !== null && (
        <p>
          <Link to={href(info.startId)} className="font-medium text-brand-700 underline dark:text-brand-100">
            {startName ? t('share.start', { name: startName }) : t('share.people')}
          </Link>
        </p>
      )}
      <OnThisDay queryKey={['share', token]} load={api.onThisDay} />
    </div>
  )
}

function SharePeople({ token }: { token: string }) {
  const { t } = useTranslation()
  const href = usePersonHref()
  const [query, setQuery] = useState('')
  const q = useDebounced(query.trim())
  const list = useQuery({
    queryKey: ['share', token, 'persons', q],
    queryFn: ({ signal }) => shareApi(token).persons(q, signal),
  })
  const items = list.data?.items ?? []
  return (
    <div className="max-w-3xl space-y-5">
      <PageHeading title={t('share.people')}>{t('share.people')}</PageHeading>
      <search>
        <label htmlFor="share-search" className="block text-sm font-medium">
          {t('share.search')}
        </label>
        <input
          id="share-search"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-describedby="share-status"
          className="mt-1 block min-h-11 w-full rounded-lg border border-slate-400 bg-white px-3 text-base dark:border-slate-600 dark:bg-slate-900"
        />
      </search>
      <p id="share-status" role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {list.isPending
          ? t('common.searching')
          : list.isError
            ? t('common.loadFailed')
            : [
                t('share.count', { count: list.data.total }),
                list.data.total > items.length ? t('share.moreResults', { shown: items.length }) : '',
              ]
                .filter(Boolean)
                .join(' ')}
      </p>
      {items.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
          {items.map((p) => (
            <li key={p.id}>
              <Link to={href(p.id)} className="block min-h-11 px-4 py-2 hover:bg-slate-50 dark:hover:bg-slate-900">
                <span className="font-medium">{fullName(p) ?? t('person.unknown')}</span>
                {lifespan(p) && <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">{lifespan(p)}</span>}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function PersonLink({ person }: { person: PersonRef | null }) {
  const { t } = useTranslation()
  const href = usePersonHref()
  if (!person) return <span className="text-slate-600 dark:text-slate-400">{t('person.unknownParent')}</span>
  return (
    <>
      <Link to={href(person.id)} className="font-medium text-brand-700 underline dark:text-brand-100">
        {fullName(person) ?? t('person.unknown')}
      </Link>
      {lifespan(person) && <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">{lifespan(person)}</span>}
    </>
  )
}

function SharePerson({ token }: { token: string }) {
  const { t } = useTranslation()
  const id = Number(useParams().id)
  const person = useQuery({
    queryKey: ['share', token, 'person', id],
    queryFn: ({ signal }) => shareApi(token).person(id, signal),
  })
  if (person.isError) {
    return (
      <p role="alert" className="font-medium">
        {person.error instanceof ApiError && person.error.status === 404 ? t('notFound.title') : t('common.loadFailed')}
      </p>
    )
  }
  if (person.isPending) return <p role="status">{t('app.loading')}</p>
  const p = person.data
  const name = fullName(p) ?? t('person.unknown')
  const birth = p.events.find((e) => e.type === 'BIRT')?.date
  const death = p.events.find((e) => e.type === 'DEAT')?.date
  const life = lifespan({
    birthDate: birth?.normalized || birth?.raw || '',
    deathDate: death?.normalized || death?.raw || '',
  })
  const others = (f: Family) => (f.partner1?.id === p.id ? f.partner2 : f.partner1)

  return (
    <article className="max-w-3xl space-y-8">
      <header className="space-y-2">
        <PageHeading title={name}>{name}</PageHeading>
        <p className="text-slate-700 dark:text-slate-300">
          {[t(`sex.${p.sex}`), life, t(p.living ? 'person.living' : 'person.deceased', { context: p.sex })].filter(Boolean).join(' · ')}
        </p>
        <Link
          to={`/share/${token}/tree?root=${p.id}`}
          className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100"
        >
          {t('share.openTree')}
        </Link>
      </header>
      <section className="space-y-3">
        <h2 className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
          {t('person.events')}
        </h2>
        {p.living && p.events.length === 0 ? (
          <p className="text-slate-600 dark:text-slate-400">{t('share.privateDetails')}</p>
        ) : (
          <Timeline items={timelineItems(p.events, p.partnerFamilies)} personId={p.id} />
        )}
      </section>
      <section className="space-y-3">
        <h2 className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
          {t('person.parentsAndSiblings')}
        </h2>
        {p.parentFamilies.length === 0 && <p className="text-slate-600 dark:text-slate-400">{t('person.noParents')}</p>}
        {p.parentFamilies.map((f) => (
          <div key={f.id} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="font-semibold">{t('person.parents')}</h3>
            <ul className="mt-1 space-y-1">
              <li>
                <PersonLink person={f.partner1} />
              </li>
              <li>
                <PersonLink person={f.partner2} />
              </li>
            </ul>
            {f.children.filter((c) => c.person.id !== p.id).length > 0 && (
              <>
                <h3 className="mt-3 font-semibold">{t('person.siblings')}</h3>
                <ul className="mt-1 space-y-1">
                  {f.children
                    .filter((c) => c.person.id !== p.id)
                    .map((c) => (
                      <li key={c.person.id}>
                        <PersonLink person={c.person} />
                      </li>
                    ))}
                </ul>
              </>
            )}
          </div>
        ))}
      </section>
      <section className="space-y-3">
        <h2 className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
          {t('person.partnersAndChildren')}
        </h2>
        {p.partnerFamilies.length === 0 && (
          <p className="text-slate-600 dark:text-slate-400">{t('person.noPartners')}</p>
        )}
        {p.partnerFamilies.map((f) => (
          <div key={f.id} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="font-semibold">
              <PersonLink person={others(f)} />
            </h3>
            {f.children.length > 0 ? (
              <ul className="mt-2 space-y-1" aria-label={t('person.children')}>
                {f.children.map((c) => (
                  <li key={c.person.id}>
                    <PersonLink person={c.person} />
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">{t('person.noChildren')}</p>
            )}
          </div>
        ))}
      </section>
      {p.notes && (
        <section className="space-y-3">
          <h2 className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
            {t('person.notes')}
          </h2>
          <p className="whitespace-pre-line">{p.notes}</p>
        </section>
      )}
    </article>
  )
}

const shareViews: TreeView[] = ['pedigree', 'descendants', 'hourglass']

function ShareTree({ token, startId }: { token: string; startId: number | null }) {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const rootId = Number(params.get('root')) || startId
  const view = (shareViews.includes(params.get('view') as TreeView) ? params.get('view') : 'hourglass') as TreeView
  const mode = params.get('mode') === 'list' ? 'list' : 'chart'
  const generations = 3
  const up = view === 'descendants' ? 0 : generations
  const down = view === 'pedigree' ? 0 : generations
  const set = (k: string, v: string) =>
    setParams(
      (p) => {
        p.set(k, v)
        return p
      },
      { replace: true },
    )
  const tree = useQuery({
    queryKey: ['share', token, 'tree', rootId, up, down],
    queryFn: ({ signal }) => shareApi(token).tree(rootId ?? 0, { up, down }, signal),
    enabled: rootId !== null,
  })
  const index = useMemo(() => (tree.data ? indexGraph(tree.data) : null), [tree.data])
  const layout = useMemo(
    () => (index ? layoutTree(index, { view, generations, bloodOnly: false }) : null),
    [index, view],
  )
  const root = index?.persons.get(index.rootId)
  const title = root ? t('tree.titleFor', { name: fullName(root) ?? t('person.unknown') }) : t('share.tree')

  return (
    <div className="space-y-4">
      <PageHeading title={title}>{title}</PageHeading>
      <div className="grid gap-3 sm:grid-cols-2 sm:items-end">
        <SelectField label={t('tree.view')} value={view} onChange={(e) => set('view', e.target.value)}>
          {shareViews.map((v) => (
            <option key={v} value={v}>
              {t(`tree.views.${v}`)}
            </option>
          ))}
        </SelectField>
        <SelectField label={t('tree.display')} value={mode} onChange={(e) => set('mode', e.target.value)}>
          <option value="chart">{t('tree.mode.chart')}</option>
          <option value="list">{t('tree.mode.list')}</option>
        </SelectField>
      </div>
      <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {rootId === null
          ? t('tree.empty')
          : tree.isPending
            ? t('app.loading')
            : tree.isError
              ? t('common.loadFailed')
              : t('tree.count', { count: index?.persons.size ?? 0 })}
      </p>
      {index &&
        layout &&
        (mode === 'chart' ? (
          <div className="h-[65dvh] min-h-[420px] overflow-hidden rounded-xl border border-slate-200 dark:border-slate-800">
            <TreeCanvas
              layout={layout}
              index={index}
              selectedId={null}
              onSelect={(id) => set('root', String(id))}
              onCenter={(id) => set('root', String(id))}
            />
          </div>
        ) : (
          <TreeList index={index} view={view} generations={generations} bloodOnly={false} />
        ))}
    </div>
  )
}
