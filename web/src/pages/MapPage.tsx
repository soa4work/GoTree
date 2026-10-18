import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useEffect, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getMapData } from '../api/endpoints'
import type { PersonRef } from '../api/types'
import { Button } from '../components/Button'
import { SelectField } from '../components/Field'
import { PageHeading } from '../components/PageHeading'
import { fullName } from '../lib/people'
import { MissingPlaces } from '../map/MissingPlaces'
import { useUnmappedPlaces } from '../map/useUnmappedPlaces'
import { positionsAt, yearRange } from '../map/positions'
import { PersonPicker } from '../people/PersonPicker'

// Leaflet is large and only this page needs it.
const MapView = lazy(() => import('../map/MapView'))

type Scope = 'all' | 'ancestors' | 'descendants'

export function MapPage() {
  const { t } = useTranslation()
  const tableId = useId()
  const [scope, setScope] = useState<Scope>('all')
  const [root, setRoot] = useState<PersonRef | null>(null)
  const [year, setYear] = useState<number | null>(null)
  const [playing, setPlaying] = useState(false)
  const ready = scope === 'all' || root !== null
  const data = useQuery({
    queryKey: ['map', scope, root?.id ?? null],
    queryFn: ({ signal }) => getMapData(scope, root?.id ?? null, signal),
    enabled: ready,
  })
  const missing = useUnmappedPlaces()
  const range = useMemo(() => (data.data ? yearRange(data.data, new Date().getFullYear()) : null), [data.data])
  const shown = range ? Math.min(Math.max(year ?? range[0], range[0]), range[1]) : null
  const positions = useMemo(
    () => (data.data && shown !== null ? positionsAt(data.data, shown) : new Map<number, number[]>()),
    [data.data, shown],
  )
  const count = [...positions.values()].reduce((n, ids) => n + ids.length, 0)

  useEffect(() => {
    if (!playing || !range || shown === null) return
    // Slower for people who asked for less motion.
    const reduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    const timer = setTimeout(
      () => (shown >= range[1] ? setPlaying(false) : setYear(shown + 1)),
      reduced ? 1200 : 350,
    )
    return () => clearTimeout(timer)
  }, [playing, shown, range])

  const places = [...positions.entries()]
    .map(([placeId, ids]) => ({ placeId, ids, place: data.data?.places[placeId] }))
    .sort((a, b) => b.ids.length - a.ids.length || (a.place?.name ?? '').localeCompare(b.place?.name ?? ''))

  return (
    <div className="max-w-5xl space-y-5">
      <div className="space-y-3">
        <PageHeading title={t('nav.map')}>{t('nav.map')}</PageHeading>
        <p className="text-slate-700 dark:text-slate-300">{t('map.intro')}</p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <SelectField
          label={t('map.scope')}
          value={scope}
          onChange={(e) => {
            setScope(e.target.value as Scope)
            setYear(null)
          }}
        >
          {(['all', 'ancestors', 'descendants'] as const).map((s) => (
            <option key={s} value={s}>
              {t(`map.scope_${s}`)}
            </option>
          ))}
        </SelectField>
        {scope !== 'all' && <PersonPicker label={t('map.root')} value={root} onChange={setRoot} />}
      </div>

      {data.isError && <p role="alert">{t('common.loadFailed')}</p>}
      {data.data && !range && (
        <div className="space-y-2">
          <p className="font-medium">{t('map.none')}</p>
          {data.data.unmapped > 0 && <p className="text-slate-700 dark:text-slate-300">{t('map.noneWhy')}</p>}
          {!!missing.data?.total && (
            <p className="text-slate-700 dark:text-slate-300">
              {t('map.noneMissing', { count: missing.data.total })}{' '}
              <a href="#map-missing-places" className="text-brand-700 underline dark:text-brand-100">
                {t('map.fixMissing')}
              </a>
            </p>
          )}
        </div>
      )}
      {data.data && range && shown !== null && (
        <>
          <div className="flex flex-wrap items-end gap-4">
            <div className="min-w-60 flex-1">
              <label htmlFor="map-year" className="block text-sm font-medium">
                {t('map.year')}: <span className="tabular-nums">{shown}</span>
              </label>
              <input
                id="map-year"
                type="range"
                min={range[0]}
                max={range[1]}
                value={shown}
                onChange={(e) => {
                  setPlaying(false)
                  setYear(Number(e.target.value))
                }}
                className="mt-1 block min-h-11 w-full accent-brand-700"
              />
            </div>
            <Button
              variant="secondary"
              onClick={() => {
                if (!playing && shown >= range[1]) setYear(range[0])
                setPlaying(!playing)
              }}
            >
              {playing ? `⏸ ${t('map.pause')}` : `▶ ${t('map.play')}`}
            </Button>
          </div>
          {/* Announcing every year while it plays would drown a screen reader. */}
          <p role={playing ? undefined : 'status'} className="text-sm text-slate-600 dark:text-slate-400">
            {t('map.status', { count, year: shown })}
          </p>
          <Suspense fallback={<p>{t('app.loading')}</p>}>
            <MapView data={data.data} positions={positions} year={shown} />
          </Suspense>
          <ul className="flex flex-wrap gap-x-6 gap-y-2 text-sm text-slate-700 dark:text-slate-300">
            <li className="flex items-center gap-2">
              <span aria-hidden="true" className="gt-marker size-6 text-xs">
                3
              </span>
              {t('map.legendMarker')}
            </li>
            <li className="flex items-center gap-2">
              <span aria-hidden="true" className="gt-marker gt-marker-approx size-6 text-xs">
                1
              </span>
              {t('map.legendApproximate')}
            </li>
            <li className="flex items-center gap-2">
              <span aria-hidden="true" className="h-1 w-8 rounded-full bg-orange-700" />
              {t('map.legendTrail')}
            </li>
          </ul>
          <p className="text-xs text-slate-600 dark:text-slate-400">{t('map.attribution')}</p>
          {data.data.unmapped > 0 && (
            <p className="text-sm text-slate-700 dark:text-slate-300">
              {t('map.unmapped', { count: data.data.unmapped })}
            </p>
          )}

          <section aria-labelledby={tableId} className="space-y-2">
            <h2 id={tableId} className="text-xl font-semibold">
              {t('map.where', { year: shown })}
            </h2>
            {/* Scrollable regions must be reachable by keyboard (WCAG 2.1.1). */}
            {/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- same reason */}
            <div className="overflow-x-auto" tabIndex={0} role="group" aria-labelledby={tableId}>
              <table className="w-full">
                <thead>
                  <tr>
                    <th scope="col" className="px-2 py-1 text-left text-sm font-semibold">
                      {t('map.place')}
                    </th>
                    <th scope="col" className="px-2 py-1 text-left text-sm font-semibold">
                      {t('map.people')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {places.map(({ placeId, ids, place }) => (
                    <tr key={placeId} className="border-t border-slate-200 align-top dark:border-slate-800">
                      <th scope="row" className="px-2 py-1 text-left font-medium">
                        {place?.name}
                        {place?.approximate && (
                          <span className="block text-xs font-normal text-slate-600 dark:text-slate-400">
                            {t('map.approximate')}
                          </span>
                        )}
                      </th>
                      <td className="px-2 py-1">
                        <ul className="flex flex-wrap gap-x-3">
                          {ids.map((id) => {
                            const p = data.data.persons[id]
                            return (
                              <li key={id}>
                                <Link
                                  to={`/people/${id}`}
                                  className="inline-flex min-h-11 items-center text-brand-700 underline dark:text-brand-100"
                                >
                                  {(p && fullName(p)) ?? t('person.unknown')}
                                </Link>
                              </li>
                            )
                          })}
                        </ul>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
      {data.data && data.data.unmapped > 0 && <MissingPlaces />}
    </div>
  )
}
