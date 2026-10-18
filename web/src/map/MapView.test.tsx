import { render } from '@testing-library/react'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import type { MapData } from '../api/types'
import i18n from '../i18n'
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

describe('map in Russian', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('ru')
  })
  afterAll(async () => {
    await i18n.changeLanguage('en')
  })

  it('translates the zoom buttons and drops the English attribution title', () => {
    const data: MapData = { places: {}, persons: {}, tracks: [], unmapped: 0 }
    const { container } = render(<MapView data={data} positions={new Map()} year={1900} />)
    const zoomIn = container.querySelector('.leaflet-control-zoom-in')
    const zoomOut = container.querySelector('.leaflet-control-zoom-out')
    expect(zoomIn?.getAttribute('title')).toBe('Приблизить')
    expect(zoomIn?.getAttribute('aria-label')).toBe('Приблизить')
    expect(zoomOut?.getAttribute('title')).toBe('Отдалить')
    expect(container.querySelector('[title="A JavaScript library for interactive maps"]')).toBeNull()
  })
})
