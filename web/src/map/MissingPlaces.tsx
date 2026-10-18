import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getAuthState } from '../api/client'
import { updatePlace } from '../api/endpoints'
import type { UnmappedPlace } from '../api/types'
import { Button } from '../components/Button'
import { TextField } from '../components/TextField'
import { lookupPlace, parseCoordinate, type GeoCandidate } from './nominatim'
import { unmappedPlacesKey, useUnmappedPlaces } from './useUnmappedPlaces'
import { errorText } from '../lib/errors'

/**
 * Places used by dated events that have no coordinates, each with a quick
 * latitude/longitude entry and an optional, per-click OpenStreetMap lookup.
 */
export function MissingPlaces() {
  const { t } = useTranslation()
  const headingId = useId()
  const [saved, setSaved] = useState('')
  const places = useUnmappedPlaces()
  const auth = useQuery({
    queryKey: ['auth'],
    queryFn: ({ signal }) => getAuthState(signal),
    staleTime: Infinity,
  })
  const readOnly = auth.data?.tree?.role === 'viewer'
  if (!places.data?.places.length && !saved) return null
  const { total = 0, places: list = [] } = places.data ?? {}
  return (
    <section aria-labelledby={headingId} id="map-missing-places" className="space-y-3">
      <h2 id={headingId} className="text-xl font-semibold">
        {t('map.missingTitle')}
      </h2>
      <p className="text-sm text-slate-700 dark:text-slate-300">{readOnly ? t('map.viewerNote') : t('map.missingIntro')}</p>
      {!readOnly && <p className="text-sm text-slate-700 dark:text-slate-300">{t('map.lookupPrivacy')}</p>}
      {total > list.length && (
        <p className="text-sm text-slate-700 dark:text-slate-300">{t('map.missingTruncated', { total, shown: list.length })}</p>
      )}
      <p role="status" className="text-sm font-medium text-slate-800 dark:text-slate-200">
        {saved && t('map.saved', { name: saved })}
      </p>
      <ul className="space-y-3">
        {list.map((p) => (
          <li key={p.id} className="rounded-lg border border-slate-300 p-3 dark:border-slate-700">
            <PlaceCoordinates place={p} readOnly={readOnly} onSaved={() => setSaved(p.fullName)} />
          </li>
        ))}
      </ul>
    </section>
  )
}

function PlaceCoordinates({ place, readOnly, onSaved }: { place: UnmappedPlace; readOnly: boolean; onSaved: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [lat, setLat] = useState('')
  const [lng, setLng] = useState('')
  const [error, setError] = useState<{ field: 'lat' | 'lng'; message: string } | undefined>()
  const [candidates, setCandidates] = useState<GeoCandidate[] | null>(null)
  const [looking, setLooking] = useState(false)
  const [lookupError, setLookupError] = useState(false)

  const save = useMutation({
    mutationFn: (c: { lat: number; lng: number }) =>
      updatePlace(place.id, {
        parentId: place.parentId,
        name: place.name,
        placeType: place.placeType,
        notes: place.notes,
        lat: c.lat,
        lng: c.lng,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['map'] })
      void queryClient.invalidateQueries({ queryKey: unmappedPlacesKey })
      onSaved()
      void queryClient.invalidateQueries({ queryKey: ['places'] })
    },
  })

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const la = parseCoordinate(lat)
    const ln = parseCoordinate(lng)
    if (la === null || ln === null)
      return setError({
        field: la === null ? 'lat' : 'lng',
        message: t('map.coordsBoth'),
      })
    if (la < -90 || la > 90) return setError({ field: 'lat', message: t('map.latRange') })
    if (ln < -180 || ln > 180) return setError({ field: 'lng', message: t('map.lngRange') })
    setError(undefined)
    save.mutate({ lat: la, lng: ln })
  }

  async function lookup() {
    if (looking) return
    setLooking(true)
    setLookupError(false)
    try {
      setCandidates(await lookupPlace(place.fullName))
    } catch {
      setCandidates(null)
      setLookupError(true)
    } finally {
      setLooking(false)
    }
  }

  return (
    <div className="space-y-2">
      <p className="font-medium">
        {place.fullName}{' '}
        <span className="text-sm font-normal text-slate-600 dark:text-slate-400">
          {t('map.placeEvents', { count: place.events })}
        </span>
      </p>
      {!readOnly && (
        <form onSubmit={submit} noValidate className="flex flex-wrap items-end gap-3">
          <TextField
            label={t('map.lat')}
            className="w-36"
            inputMode="decimal"
            value={lat}
            onChange={(e) => setLat(e.target.value)}
            error={error?.field === 'lat' ? error.message : undefined}
          />
          <TextField
            label={t('map.lng')}
            className="w-36"
            inputMode="decimal"
            value={lng}
            onChange={(e) => setLng(e.target.value)}
            error={error?.field === 'lng' ? error.message : undefined}
          />
          <Button type="submit" busy={save.isPending}>
            {t('map.saveCoords')}
            <span className="sr-only"> — {place.fullName}</span>
          </Button>
          <Button type="button" variant="secondary" busy={looking} onClick={() => void lookup()}>
            {t('map.lookup')}
            <span className="sr-only"> — {place.fullName}</span>
          </Button>
        </form>
      )}
      {save.isError && (
        <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
          {t('map.saveFailed', { message: errorText(save.error, t) })}
        </p>
      )}
      {lookupError && (
        <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
          {t('map.lookupFailed')}
        </p>
      )}
      {candidates && (
        <div role="status">
          {candidates.length === 0 ? (
            <p className="text-sm">{t('map.lookupNone')}</p>
          ) : (
            <ul className="space-y-1">
              {candidates.map((c) => (
                <li key={`${c.lat},${c.lng},${c.name}`} className="flex flex-wrap items-center gap-2 text-sm">
                  <span className="min-w-0 flex-1">{c.name}</span>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => {
                      setLat(String(c.lat))
                      setLng(String(c.lng))
                      setError(undefined)
                    }}
                  >
                    {t('map.useCandidate')}
                    <span className="sr-only"> — {c.name}</span>
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <p className="mt-1 text-xs text-slate-600 dark:text-slate-400">{t('map.lookupAttribution')}</p>
        </div>
      )}
    </div>
  )
}
