import i18n from '../i18n'

export interface GeoCandidate {
  name: string
  lat: number
  lng: number
}

interface NominatimHit {
  display_name?: string
  lat?: string
  lon?: string
}

/** Nominatim allows one request per second for the whole app, not per place. */
export const LOOKUP_GAP_MS = 1100
let nextSlot = 0

/** Only for tests: forgets the reserved slots. */
export function resetLookupThrottle() {
  nextSlot = 0
}

/** Waits for the app-wide turn; the slot is reserved synchronously, so parallel calls queue up. */
function waitTurn(signal?: AbortSignal): Promise<void> {
  const now = Date.now()
  const at = Math.max(now, nextSlot)
  nextSlot = at + LOOKUP_GAP_MS
  if (at === now) return Promise.resolve()
  return new Promise((resolve, reject) => {
    // An aborted call gives its slot back when nobody queued behind it.
    const release = () => {
      if (nextSlot === at + LOOKUP_GAP_MS) nextSlot = at
    }
    if (signal?.aborted) {
      release()
      return reject(signal.reason)
    }
    const onAbort = () => {
      clearTimeout(timer)
      release()
      reject(signal?.reason)
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, at - now)
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

/**
 * Asks OpenStreetMap's Nominatim for up to five matches of one place name.
 * Only ever called from an explicit click on one place: Nominatim's usage
 * policy allows at most one request per second and forbids bulk geocoding.
 */
export async function lookupPlace(query: string, signal?: AbortSignal): Promise<GeoCandidate[]> {
  await waitTurn(signal)
  const params = new URLSearchParams({
    format: 'jsonv2',
    limit: '5',
    q: query,
    'accept-language': i18n.resolvedLanguage ?? 'ru',
  })
  const res = await fetch(`https://nominatim.openstreetmap.org/search?${params}`, {
    signal,
    // The app's Referrer-Policy is same-origin; Nominatim's usage policy wants the app identified.
    referrerPolicy: 'strict-origin-when-cross-origin',
    headers: { Accept: 'application/json' },
  })
  if (!res.ok) throw new Error(res.statusText || String(res.status))
  const hits = (await res.json()) as NominatimHit[]
  return hits.flatMap((h) => {
    const lat = Number(h.lat)
    const lng = Number(h.lon)
    return h.display_name && Number.isFinite(lat) && Number.isFinite(lng) ? [{ name: h.display_name, lat, lng }] : []
  })
}

/** Parses a coordinate typed by a person; both "51.34" and "51,34" are fine. */
export function parseCoordinate(text: string): number | null {
  const t = text.trim().replace(',', '.')
  if (t === '' || !/^[-+]?\d+(\.\d+)?$|^[-+]?\.\d+$/.test(t)) return null
  return Number(t)
}
