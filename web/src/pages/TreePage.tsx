import { useQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'
import { ApiError, getAuthState } from '../api/client'
import { getTree, listPersons } from '../api/endpoints'
import type { PersonRef } from '../api/types'
import { SelectField } from '../components/Field'
import { OnboardingCard } from '../components/OnboardingCard'
import { PageHeading } from '../components/PageHeading'
import { lgQuery, useMediaQuery } from '../hooks/useMediaQuery'
import { fullName } from '../lib/people'
import { PersonPicker } from '../people/PersonPicker'
import { layoutTree, type TreeView } from '../tree/layout'
import { indexGraph } from '../tree/model'
import { PersonPanel } from '../tree/PersonPanel'
import { TreeCanvas } from '../tree/TreeCanvas'
import { TreeList } from '../tree/TreeList'
import type { TreeLook } from '../tree/nodeTypes'

const views: TreeView[] = ['pedigree', 'descendants', 'hourglass', 'family']
const legacyRootKey = 'gotree.treeRoot'
const lookKey = 'gotree.treeLook'

function storedLook(): TreeLook {
  try {
    return localStorage.getItem(lookKey) === 'plain' ? 'plain' : 'leafy'
  } catch {
    return 'leafy'
  }
}

function saveLook(look: TreeLook) {
  try {
    localStorage.setItem(lookKey, look)
  } catch {
    // Without storage the choice lasts until the page is reloaded.
  }
}

function remembered(key: string): number | null {
  try {
    const v = Number(localStorage.getItem(key))
    return v > 0 ? v : null
  } catch {
    return null
  }
}

function remember(key: string, id: number) {
  try {
    localStorage.setItem(key, String(id))
  } catch {
    // Storage can be unavailable (private mode); the URL still works.
  }
}

function forget(key: string) {
  try {
    localStorage.removeItem(key)
    // The key used before the root was remembered per tree.
    localStorage.removeItem(legacyRootKey)
  } catch {
    // Nothing was stored.
  }
}

export function TreePage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const genId = useId()
  const [look, setLook] = useState<TreeLook>(storedLook)
  const modeName = useId()
  const optionsId = useId()
  const [optionsOpen, setOptionsOpen] = useState(false)
  const wide = useMediaQuery(lgQuery)

  const view = (views.includes(params.get('view') as TreeView) ? params.get('view') : 'pedigree') as TreeView
  const generations = Math.min(10, Math.max(1, Number(params.get('gen')) || 4))
  const bloodOnly = params.get('blood') === '1'
  const mode = params.get('mode') === 'list' ? 'list' : 'chart'
  const rootParam = Number(params.get('root')) || null
  const selectedId = Number(params.get('sel')) || null

  const set = useCallback(
    (patch: Record<string, string | null>, replace = true) =>
      setParams(
        (p) => {
          for (const [k, v] of Object.entries(patch)) {
            if (v === null) p.delete(k)
            else p.set(k, v)
          }
          return p
        },
        { replace },
      ),
    [setParams],
  )

  // The remembered root belongs to one tree: a replaced tree has other people.
  const auth = useQuery({
    queryKey: ['auth'],
    queryFn: ({ signal }) => getAuthState(signal),
    staleTime: Infinity,
  })
  const rootKey = `gotree.treeRoot.${auth.data?.tree?.id ?? 0}`
  // Read on every render, not once: centering on someone remembers them, and
  // following the Tree link again (same page instance) must start there. A
  // root that 404s is forgotten in the tree query before its error renders.
  const stored = remembered(rootKey)

  // Without a root in the URL: the last one viewed, else the first person.
  const source = rootParam !== null ? 'url' : stored !== null ? 'stored' : 'first'
  const fallback = useQuery({
    queryKey: ['persons', 'first'],
    queryFn: ({ signal }) => listPersons('', 1, 0, signal),
    enabled: rootParam === null && stored === null,
    staleTime: 0,
  })
  const rootId = rootParam ?? stored ?? fallback.data?.items[0]?.id ?? null

  const up = view === 'descendants' ? 0 : view === 'family' ? 1 : generations
  const down = view === 'pedigree' ? 0 : view === 'family' ? 1 : generations
  const tree = useQuery({
    queryKey: ['tree', rootId, up, down, view === 'family'],
    queryFn: async ({ signal }) => {
      try {
        return await getTree(rootId!, { up, down, siblings: view === 'family' }, signal)
      } catch (e) {
        // A person that is gone must not be remembered as the root.
        if (e instanceof ApiError && e.status === 404 && remembered(rootKey) === rootId) forget(rootKey)
        throw e
      }
    },
    enabled: rootId !== null,
  })

  const missing = tree.error instanceof ApiError && tree.error.status === 404
  const retried = useRef(false)
  useEffect(() => {
    if (!missing || source !== 'first' || retried.current) return
    retried.current = true
    void fallback.refetch()
  }, [missing, source, fallback])

  // Only a root that loaded is worth remembering.
  const loadedRoot = tree.isSuccess ? tree.data.rootId : null
  useEffect(() => {
    if (loadedRoot !== null) remember(rootKey, loadedRoot)
  }, [loadedRoot, rootKey])

  const index = useMemo(() => (tree.data ? indexGraph(tree.data) : null), [tree.data])
  const layout = useMemo(
    () => (index ? layoutTree(index, { view, generations, bloodOnly }) : null),
    [index, view, generations, bloodOnly],
  )
  const root = index?.persons.get(index.rootId) ?? null
  const selected = selectedId !== null ? (index?.persons.get(selectedId) ?? null) : null

  const centerOn = useCallback((id: number) => set({ root: String(id), sel: null }, false), [set])
  const onSelect = useCallback((id: number) => set({ sel: String(id) }), [set])
  const title = root ? t('tree.titleFor', { name: fullName(root) ?? t('person.unknown') }) : t('nav.tree')

  const modeSwitch = (
    <fieldset className="flex min-w-0 flex-1 gap-1 rounded-lg border border-slate-300 p-1 md:max-w-sm dark:border-slate-700">
      <legend className="sr-only">{t('tree.display')}</legend>
      {(['chart', 'list'] as const).map((m) => (
        <label
          key={m}
          className="flex min-h-11 flex-1 cursor-pointer items-center justify-center rounded-md font-medium has-[:checked]:bg-brand-700 has-[:checked]:text-white has-[:focus-visible]:outline has-[:focus-visible]:outline-3 has-[:focus-visible]:outline-brand-500"
        >
          <input
            type="radio"
            name={modeName}
            className="sr-only"
            checked={mode === m}
            onChange={() => set({ mode: m === 'list' ? 'list' : null })}
          />
          {t(`tree.mode.${m}`)}
        </label>
      ))}
    </fieldset>
  )

  if (rootId === null && !fallback.isPending) {
    return (
      <div className="max-w-3xl space-y-4">
        <PageHeading title={t('nav.tree')}>{t('nav.tree')}</PageHeading>
        <OnboardingCard />
      </div>
    )
  }

  if (missing && source === 'url') {
    return (
      <div className="max-w-3xl space-y-4">
        <PageHeading title={t('nav.tree')}>{t('nav.tree')}</PageHeading>
        <p role="alert" className="font-medium">
          {t('tree.rootMissing')}
        </p>
        <Link
          to="/tree"
          className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100"
        >
          {t('tree.showFirst')}
        </Link>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeading title={title}>{title}</PageHeading>
        {rootId !== null && (
          <Link
            to={`/chart?root=${rootId}`}
            className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100"
          >
            {t('chart.printChart')}
          </Link>
        )}
      </div>

      <div className="flex items-stretch gap-2">
        <button
          type="button"
          aria-expanded={optionsOpen}
          aria-controls={optionsId}
          onClick={() => setOptionsOpen((o) => !o)}
          className="inline-flex min-h-11 shrink-0 items-center gap-2 rounded-lg border border-slate-400 px-4 font-medium hover:bg-slate-100 md:hidden dark:border-slate-600 dark:hover:bg-slate-800"
        >
          {t('tree.options')}
          <span aria-hidden="true">{optionsOpen ? '▴' : '▾'}</span>
        </button>
        {modeSwitch}
      </div>

      <div
        id={optionsId}
        className={`${optionsOpen ? 'grid' : 'hidden'} grid-cols-2 gap-x-3 gap-y-4 md:grid md:grid-cols-[1fr_1fr_1fr_auto] md:items-end`}
      >
        <SelectField label={t('tree.view')} value={view} onChange={(e) => set({ view: e.target.value })}>
          {views.map((v) => (
            <option key={v} value={v}>
              {t(`tree.views.${v}`)}
            </option>
          ))}
        </SelectField>
        <div>
          <label htmlFor={genId} className="block text-sm font-medium">
            {t('tree.generations')}
          </label>
          <div className="mt-1 flex min-h-11 items-center gap-3">
            <input
              id={genId}
              type="range"
              min={1}
              max={10}
              value={view === 'family' ? 1 : generations}
              disabled={view === 'family'}
              onChange={(e) => set({ gen: e.target.value })}
              className="w-full accent-brand-700"
            />
            <output htmlFor={genId} className="w-6 text-right font-medium">
              {view === 'family' ? 1 : generations}
            </output>
          </div>
        </div>
        <div className="col-span-2 md:col-span-1">
          <PersonPicker
            label={t('tree.centerOn')}
            value={null}
            onChange={(p: PersonRef | null) => p && centerOn(p.id)}
          />
        </div>
        <div className="col-span-2 flex flex-col gap-2 md:col-span-1">
          <label className="flex min-h-11 items-center gap-2">
            <input
              type="checkbox"
              className="size-5"
              checked={bloodOnly}
              disabled={view === 'pedigree'}
              onChange={(e) => set({ blood: e.target.checked ? '1' : null })}
            />
            {t('tree.bloodOnly')}
          </label>
          {mode === 'chart' && (
            <label className="flex min-h-11 items-center gap-2">
              <input
                type="checkbox"
                className="size-5"
                checked={look === 'leafy'}
                onChange={(e) => {
                  const next = e.target.checked ? 'leafy' : 'plain'
                  setLook(next)
                  saveLook(next)
                }}
              />
              {t('tree.leafy')}
            </label>
          )}
        </div>
      </div>

      <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {tree.isPending
          ? t('app.loading')
          : tree.isError
            ? t('common.loadFailed')
            : [t('tree.count', { count: index?.persons.size ?? 0 }), tree.data?.truncated ? t('tree.truncated') : '']
                .filter(Boolean)
                .join(' ')}
      </p>

      {index && layout && (
        <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
          {mode === 'chart' ? (
            <div className="relative h-[65dvh] min-h-[420px] overflow-hidden rounded-xl border border-slate-200 md:h-[calc(100dvh-20rem)] dark:border-slate-800">
              <TreeCanvas
                layout={layout}
                index={index}
                selectedId={selectedId}
                onSelect={onSelect}
                onCenter={centerOn}
                look={look}
                sheetOpen={selected !== null && !wide}
              />
              {selected && !wide && (
                <PersonPanel
                  sheet
                  person={selected}
                  isRoot={selected.id === index.rootId}
                  onCenter={centerOn}
                  onClose={() => set({ sel: null })}
                />
              )}
            </div>
          ) : (
            <TreeList index={index} view={view} generations={generations} bloodOnly={bloodOnly} />
          )}
          {selected && (wide || mode === 'list') && (
            <PersonPanel
              person={selected}
              isRoot={selected.id === index.rootId}
              onCenter={centerOn}
              onClose={() => set({ sel: null })}
            />
          )}
        </div>
      )}
    </div>
  )
}
