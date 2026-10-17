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

/**
 * Asks OpenStreetMap's Nominatim for up to five matches of one place name.
 * Only ever called from an explicit click on one place: Nominatim's usage
 * policy allows at most one request per second and forbids bulk geocoding.
 */
export async function lookupPlace(query: string, signal?: AbortSignal): Promise<GeoCandidate[]> {
  const params = new URLSearchParams({ format: 'jsonv2', limit: '5', q: query })
  const res = await fetch(`https://nominatim.openstreetmap.org/search?${params}`, {
    signal,
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
