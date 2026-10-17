import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { MapData } from '../api/types'
import { arc, trailsUntil } from './positions'

interface Props {
  data: MapData
  positions: Map<number, number[]>
  year: number
}

/** How long a move stays clearly visible before it fades into the background. */
const FADE_YEARS = 40

/**
 * The Leaflet map: one numbered marker per place, sized by how many people
 * were there, and arcs for the moves that brought them there, fading with
 * age. It is the visual companion of the table next to it, which holds the
 * same information for everyone.
 */
export default function MapView({ data, positions, year }: Props) {
  const { t } = useTranslation()
  const box = useRef<HTMLDivElement>(null)
  const map = useRef<L.Map | null>(null)
  const layer = useRef<L.LayerGroup | null>(null)

  useEffect(() => {
    if (!box.current) return
    const m = L.map(box.current, { scrollWheelZoom: false, keyboard: true })
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 18,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
    }).addTo(m)
    layer.current = L.layerGroup().addTo(m)
    map.current = m
    return () => {
      m.remove()
      map.current = null
    }
  }, [])

  // Fit to all places of the data once, not to each year, so the map holds
  // still while it plays.
  useEffect(() => {
    const places = Object.values(data.places)
    if (!map.current || places.length === 0) return
    map.current.fitBounds(L.latLngBounds(places.map((p) => [p.lat, p.lng])), { padding: [24, 24], maxZoom: 10 })
  }, [data])

  useEffect(() => {
    const group = layer.current
    if (!group) return
    group.clearLayers()
    for (const trail of trailsUntil(data, year, positions)) {
      const from = data.places[trail.from]
      const to = data.places[trail.to]
      if (!from || !to) continue
      const fresh = Math.max(0, 1 - (year - trail.last) / FADE_YEARS)
      L.polyline(arc([from.lat, from.lng], [to.lat, to.lng]), {
        color: '#c2410c',
        weight: 2 + Math.min(4, trail.count),
        opacity: 0.3 + 0.6 * fresh,
        dashArray: fresh > 0 ? undefined : '4 6',
        lineCap: 'round',
        interactive: false,
      }).addTo(group)
    }
    for (const [placeId, people] of positions) {
      const place = data.places[placeId]
      if (!place) continue
      const size = Math.round(24 + Math.sqrt(people.length) * 8)
      // Leaflet inserts a string tooltip as HTML, and place names come from
      // users and GEDCOM files; a DOM node with textContent stays plain text.
      const label = document.createElement('div')
      label.textContent = `${place.name}: ${people.length}`
      L.marker([place.lat, place.lng], {
        keyboard: false,
        zIndexOffset: people.length,
        icon: L.divIcon({
          className: '',
          html: `<span class="gt-marker${place.approximate ? ' gt-marker-approx' : ''}" style="width:${size}px;height:${size}px">${people.length}</span>`,
          iconSize: [size, size],
          iconAnchor: [size / 2, size / 2],
        }),
      })
        .bindTooltip(label, { direction: 'top', offset: [0, -size / 2] })
        .addTo(group)
    }
  }, [positions, data, year])

  return <div ref={box} aria-label={t('map.mapLabel', { year })} role="region" className="h-[60dvh] min-h-80 w-full rounded-xl" />
}
