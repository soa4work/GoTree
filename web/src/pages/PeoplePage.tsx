import { useInfiniteQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { listPersons, type PersonFilter } from '../api/endpoints'
import { Button } from '../components/Button'
import { SelectField } from '../components/Field'
import { OnboardingCard } from '../components/OnboardingCard'
import { PageHeading } from '../components/PageHeading'
import { useDebounced } from '../hooks/useDebounced'
import { useShortcuts } from '../hooks/useShortcuts'
import { fullName, lifespan } from '../lib/people'
import { Avatar } from '../media/Avatar'
import { PersonDialog } from '../people/PersonDialog'

const pageSize = 50

// The default order (surname, or best match when searching) is the empty choice.
const sorts = ['birth', '-birth', 'updated'] as const
const sexes = ['M', 'F', 'X', 'U'] as const
const sortLabel = { birth: 'sort_birth', '-birth': 'sort_birthDesc', updated: 'sort_updated' } as const

// Anything else in the address is dropped, so a stale or hand-edited link still works.
function readFilter(params: URLSearchParams): PersonFilter {
  const sort = sorts.find((v) => v === params.get('sort'))
  const sex = sexes.find((v) => v === params.get('sex'))
  const living = params.get('living')
  return { sort, sex, living: living === '1' ? true : living === '0' ? false : undefined }
}

export function PeoplePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const [query, setQuery] = useState(params.get('q') ?? '')
  const q = useDebounced(query.trim())
  const creating = params.get('new') === '1'
  const { sort, sex, living } = readFilter(params)
  const filtersActive = sort !== undefined || sex !== undefined || living !== undefined
  const livingValue = living === undefined ? '' : living ? '1' : '0'

  const people = useInfiniteQuery({
    queryKey: ['persons', 'list', q, sort, sex, living],
    queryFn: ({ pageParam, signal }) => listPersons(q, pageSize, pageParam, signal, { sort, sex, living }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < last.total ? loaded : undefined
    },
  })
  const items = people.data?.pages.flatMap((p) => p.items) ?? []
  const total = people.data?.pages[0]?.total ?? 0

  const setCreating = (on: boolean) =>
    setParams(
      (p) => {
        if (on) p.set('new', '1')
        else p.delete('new')
        return p
      },
      { replace: true },
    )

  const setFilter = (key: 'sort' | 'sex' | 'living', value: string) =>
    setParams(
      (p) => {
        if (value) p.set(key, value)
        else p.delete(key)
        return p
      },
      { replace: true },
    )

  useShortcuts({
    '/': () => document.getElementById('people-search')?.focus(),
    n: () => setCreating(true),
  })

  let status = t('common.searching')
  if (!people.isPending) status = q || filtersActive ? t('people.found', { count: total }) : t('people.count', { count: total })

  return (
    <div className="max-w-3xl space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeading title={t('nav.people')}>{t('nav.people')}</PageHeading>
        <Button onClick={() => setCreating(true)}>
          + {t('person.new')}
          <span className="sr-only"> ({t('shortcuts.key', { key: 'n' })})</span>
        </Button>
      </div>

      <search>
        <label htmlFor="people-search" className="block text-sm font-medium">
          {t('people.search')}
        </label>
        <input
          id="people-search"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-describedby="people-status"
          placeholder={t('people.searchPlaceholder')}
          className="mt-1 block min-h-11 w-full rounded-lg border border-slate-400 bg-white px-3 text-base dark:border-slate-600 dark:bg-slate-900"
        />
        <div className="mt-3 grid gap-3 sm:grid-cols-3">
          <SelectField label={t('people.sortBy')} value={sort ?? ''} onChange={(e) => setFilter('sort', e.target.value)}>
            <option value="">{t(q ? 'people.sort_relevance' : 'people.sort_name')}</option>
            {sorts.map((v) => (
              <option key={v} value={v}>
                {t(`people.${sortLabel[v]}`)}
              </option>
            ))}
          </SelectField>
          <SelectField label={t('people.sexFilter')} value={sex ?? ''} onChange={(e) => setFilter('sex', e.target.value)}>
            <option value="">{t('people.all')}</option>
            {sexes.map((v) => (
              <option key={v} value={v}>
                {t(`sex.${v}`)}
              </option>
            ))}
          </SelectField>
          <SelectField label={t('people.livingFilter')} value={livingValue} onChange={(e) => setFilter('living', e.target.value)}>
            <option value="">{t('people.all')}</option>
            <option value="1">{t('people.living')}</option>
            <option value="0">{t('people.deceased')}</option>
          </SelectField>
        </div>
      </search>
      <p id="people-status" role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {people.isError ? t('common.loadFailed') : status}
      </p>

      {items.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
          {items.map((p) => (
            <li key={p.id}>
              <Link
                to={`/people/${p.id}`}
                className="flex min-h-14 items-center justify-between gap-3 px-4 py-2 hover:bg-slate-50 dark:hover:bg-slate-900"
              >
                <span className="flex items-center gap-3">
                  <Avatar person={p} size={40} />
                  <span className="font-medium">{fullName(p) ?? t('person.unknown')}</span>
                </span>
                <span className="shrink-0 text-sm text-slate-600 dark:text-slate-400">{lifespan(p)}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {!people.isPending && total === 0 && !q && !filtersActive && !creating && <OnboardingCard />}
      {!people.isPending && total === 0 && filtersActive && (
        <p className="text-slate-600 dark:text-slate-400">{t('people.noMatch')}</p>
      )}
      {people.hasNextPage && (
        <Button variant="secondary" onClick={() => void people.fetchNextPage()} busy={people.isFetchingNextPage}>
          {t('people.more')}
        </Button>
      )}

      <PersonDialog open={creating} onClose={() => setCreating(false)} onSaved={(p) => navigate(`/people/${p.id}`)} />
    </div>
  )
}
