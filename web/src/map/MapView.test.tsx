import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MapData } from '../api/types'
import MapView from './MapView'

describe('map tooltips', () => {
  it('shows a place name containing HTML as text', () => {
    const name = '<img src=x onerror="window.pwned=1"><b>Evil</b>'
    const data: MapData = {
      places: { 1: { name, lat: 51, lng: 13, approximate: false } },
      persons: {},
      tracks: [],
      unmapped: 0,
    }
    const { container } = render(<MapView data={data} positions={new Map([[1, [7]]])} year={1900} />)
    const marker = container.querySelector('.leaflet-marker-icon')
    expect(marker).not.toBeNull()
    marker!.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }))
    const tip = container.querySelector('.leaflet-tooltip')
    expect(tip).not.toBeNull()
    expect(tip!.textContent).toBe(`${name}: 1`)
    expect(tip!.querySelector('img, b')).toBeNull()
  })
})
