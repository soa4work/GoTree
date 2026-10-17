import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { listHeirlooms, thumbUrl } from '../api/endpoints'
import { Button } from '../components/Button'
import { PageHeading } from '../components/PageHeading'
import { HeirloomDialog } from '../heirlooms/HeirloomDialog'
import { useDebounced } from '../hooks/useDebounced'
import { fullName } from '../lib/people'
import { formatDate } from '../lib/dates'

export function HeirloomsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const q = useDebounced(query.trim())
  const list = useQuery({ queryKey: ['heirlooms', 'list', q], queryFn: ({ signal }) => listHeirlooms({ q }, signal) })
  const items = list.data ?? []

  return (
    <div className="max-w-3xl space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeading title={t('nav.heirlooms')}>{t('nav.heirlooms')}</PageHeading>
        <Button onClick={() => setCreating(true)}>+ {t('heirloom.new')}</Button>
      </div>
      <p className="text-slate-700 dark:text-slate-300">{t('heirloom.intro')}</p>
      <search>
        <label htmlFor="heirloom-search" className="block text-sm font-medium">
          {t('heirloom.search')}
        </label>
        <input
          id="heirloom-search"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-describedby="heirloom-status"
          className="mt-1 block min-h-11 w-full rounded-lg border border-slate-400 bg-white px-3 text-base dark:border-slate-600 dark:bg-slate-900"
        />
      </search>
      <p id="heirloom-status" role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {list.isPending
          ? t('common.searching')
          : list.isError
            ? t('common.loadFailed')
            : items.length === 0 && q === ''
              ? t('heirloom.none')
              : t('heirloom.count', { count: items.length })}
      </p>
      {items.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
          {items.map((h) => (
            <li key={h.id}>
              <Link to={`/heirlooms/${h.id}`} className="flex min-h-16 items-center gap-3 px-4 py-2 hover:bg-slate-50 dark:hover:bg-slate-900">
                {h.photoId !== null ? (
                  <img src={thumbUrl(h.photoId, 128)} alt="" loading="lazy" className="size-12 shrink-0 rounded-lg object-cover" />
                ) : (
                  <span aria-hidden="true" className="flex size-12 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-xl dark:bg-slate-800">
                    🏺
                  </span>
                )}
                <span className="min-w-0">
                  <span className="block font-medium">{h.name}</span>
                  <span className="block text-sm text-slate-600 dark:text-slate-400">
                    {[
                      t(`heirloom.kinds.${h.kind}`),
                      formatDate(h.madeDate),
                      h.holder ? t('heirloom.heldBy', { name: fullName(h.holder) ?? t('person.unknown') }) : '',
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {creating && <HeirloomDialog onClose={() => setCreating(false)} onSaved={(h) => navigate(`/heirlooms/${h.id}`)} />}
    </div>
  )
}
