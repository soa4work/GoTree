import { expect, test } from '@playwright/test'
import { expectAccessible, login, unique } from './helpers'

// A transparent 1×1 PNG, so the map never needs the internet.
const tile = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64')

test('statistics and the migration map', async ({ page }) => {
  await page.route('https://tile.openstreetmap.org/**', (route) => route.fulfill({ contentType: 'image/png', body: tile }))
  await login(page)
  const surname = unique('Wander', page)
  const city = await (await page.request.post('/api/places', { data: { name: `Bremen${surname}`, lat: 53.08, lng: 8.8 } })).json()
  const port = await (await page.request.post('/api/places', { data: { name: `NewYork${surname}`, lat: 40.71, lng: -74.0 } })).json()
  const fritz = await (await page.request.post('/api/persons', { data: { givenNames: 'Fritz', surname, sex: 'M' } })).json()
  for (const [type, date, placeId] of [
    ['BIRT', '3 MAR 1850', city.id],
    ['EMIG', '1880', port.id],
    ['DEAT', '1920', port.id],
  ]) {
    await page.request.post('/api/events', { data: { personId: fritz.id, type, date, placeId } })
  }

  await page.goto('/stats')
  await expect(page.getByRole('region', { name: 'Surnames' }).getByText(`${surname}, 1 person`)).toBeAttached()
  await expect(page.getByRole('region', { name: 'Lifespan by decade of birth' }).getByRole('table')).toBeVisible()
  await expectAccessible(page)

  await page.goto('/map')
  await page.getByLabel('Show').selectOption('descendants')
  await page.getByRole('searchbox', { name: 'Person' }).fill(`Fritz ${surname}`)
  await page.getByRole('button', { name: new RegExp(`^Fritz ${surname}`) }).click()
  const slider = page.getByRole('slider', { name: /Year/ })
  await expect(slider).toHaveAttribute('min', '1850')
  await expect(slider).toHaveAttribute('max', '1920')
  await expect(page.getByRole('row', { name: new RegExp(`Bremen${surname}.*Fritz`) })).toBeVisible()
  // One numbered marker per place, and no move yet.
  await expect(page.locator('.leaflet-marker-pane .gt-marker')).toHaveText(['1'])
  await expect(page.locator('.leaflet-overlay-pane path')).toHaveCount(0)
  await expectAccessible(page)

  await slider.fill('1900')
  await expect(page.getByRole('heading', { name: 'Who was where in 1900' })).toBeVisible()
  await expect(page.getByRole('row', { name: new RegExp(`NewYork${surname}.*Fritz`) })).toBeVisible()
  // The emigration is drawn as a trail from Bremen to New York.
  await expect(page.locator('.leaflet-overlay-pane path')).toHaveCount(1)
})

test('places without coordinates are fixed from the map page', async ({ page }) => {
  // The lookup only happens on a click, and never reaches the real service.
  let lookups = 0
  await page.route('https://nominatim.openstreetmap.org/**', (route) => {
    lookups++
    return route.fulfill({ json: [{ display_name: 'Stettin, Poland', lat: '53.43', lon: '14.55' }] })
  })
  await page.route('https://tile.openstreetmap.org/**', (route) => route.fulfill({ contentType: 'image/png', body: tile }))
  await login(page)
  const name = unique('Stettin', page)
  const place = await (await page.request.post('/api/places', { data: { name } })).json()
  const anna = await (await page.request.post('/api/persons', { data: { givenNames: 'Anna', surname: name, sex: 'F' } })).json()
  await page.request.post('/api/events', { data: { personId: anna.id, type: 'BIRT', date: '1870', placeId: place.id } })

  await page.goto('/map')
  const row = page.getByRole('listitem').filter({ hasText: name })
  await expect(row).toBeVisible()
  expect(lookups).toBe(0)
  await expectAccessible(page)

  await row.getByRole('button', { name: /Look up on OpenStreetMap/ }).click()
  await row.getByRole('button', { name: /Use these coordinates/ }).click()
  expect(lookups).toBe(1)
  await expect(row.getByLabel('Latitude')).toHaveValue('53.43')
  await row.getByRole('button', { name: /Save coordinates/ }).click()
  await expect(row).toBeHidden()
  const saved = await (await page.request.get(`/api/places/${place.id}`)).json()
  expect([saved.lat, saved.lng]).toEqual([53.43, 14.55])
})
