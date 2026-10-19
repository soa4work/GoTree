import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect, test } from './base'
import { expectAccessible, expectNoHorizontalScroll, login, unique } from './helpers'

const photoBytes = readFileSync(fileURLToPath(new URL('./fixtures/family-photo.jpg', import.meta.url)))

/**
 * The flow tests of these pages run on the desktop only; i18n.spec.ts walks the main list pages at
 * phone width. This one keeps axe and the overflow check at phone width for what that walk does not
 * visit: the detail pages, two dialogs and the share view. English, so a failure names the page.
 */
test('details, dialogs and the share view fit a phone and pass axe', { tag: '@mobile-only' }, async ({ page, browser }) => {
  test.slow()
  // Map tiles and Nominatim are outside the test.
  await page.route(/^https?:\/\/(?!127\.0\.0\.1)/, (r) => r.abort())
  await login(page)
  const api = page.request
  const surname = unique('Phone', page)

  const person = (await (
    await api.post('/api/persons', { data: { givenNames: 'Anna', surname, sex: 'F', birth: { date: '12 MAR 1890' }, death: { date: '1950' } } })
  ).json()) as { id: number }
  const residence = (await (await api.post('/api/events', { data: { personId: person.id, type: 'RESI', date: '1900' } })).json()) as { id: number }
  const source = (await (await api.post('/api/sources', { data: { title: `Kirchenbuch ${surname}` } })).json()) as { id: number }
  const cite = await api.post('/api/citations', {
    data: {
      sourceId: source.id,
      page: 'S. 12',
      quality: null,
      text: '',
      notes: '',
      links: [{ entityType: 'event', entityId: residence.id, field: '', status: '', statusReason: '' }],
    },
  })
  expect(cite.ok(), await cite.text()).toBe(true)
  const upload = await api.post('/api/media', {
    multipart: { file: { name: 'phone.jpg', mimeType: 'image/jpeg', buffer: Buffer.concat([photoBytes, Buffer.from(surname)]) } },
  })
  expect(upload.ok(), await upload.text()).toBe(true)
  const media = (await upload.json()) as { id: number }
  const link = await api.post(`/api/media/${media.id}/links`, { data: { entityType: 'person', entityId: person.id } })
  expect(link.ok(), await link.text()).toBe(true)
  const heirloom = await api.post('/api/heirlooms', {
    data: {
      name: `Ring ${surname}`,
      kind: 'jewellery',
      description: '',
      madeDate: 'FEB 1880',
      originPlaceId: null,
      currentLocation: '',
      notes: '',
      custody: [{ personId: person.id, fromDate: 'FEB 1920', toDate: '', how: 'gift', notes: '' }],
    },
  })
  expect(heirloom.ok(), await heirloom.text()).toBe(true)
  const heirloomId = ((await heirloom.json()) as { id: number }).id
  const share = await api.post('/api/share-links', { data: { label: `Cousins ${surname}`, scope: 'descendants', rootPersonId: person.id, privacy: 'deceased', expiresOn: '' } })
  expect(share.ok(), await share.text()).toBe(true)
  const { token } = (await share.json()) as { token: string }

  const check = async (p: typeof page) => {
    await expectNoHorizontalScroll(p)
    await expectAccessible(p)
  }

  for (const path of ['/members', '/accounts', `/sources/${source.id}`, `/media/${media.id}`, `/heirlooms/${heirloomId}`]) {
    await test.step(path, async () => {
      await page.goto(path)
      await expect(page.getByRole('heading', { level: 1 }).first()).toBeVisible()
      await page.waitForLoadState('networkidle')
      await check(page)
    })
  }

  await test.step('dialogs on the person page', async () => {
    await page.goto(`/people/${person.id}`)
    await expect(page.getByRole('heading', { level: 1, name: `Anna ${surname}` })).toBeVisible()
    await page.getByRole('button', { name: /^\+ Event/ }).click()
    const eventDialog = page.getByRole('dialog', { name: 'New life event' })
    await expect(eventDialog).toBeVisible()
    await check(page)
    await page.keyboard.press('Escape')
    await expect(eventDialog).toBeHidden()

    await page.getByRole('button', { name: /^Edit/ }).first().click()
    const editDialog = page.getByRole('dialog', { name: 'Edit person' })
    await expect(editDialog).toBeVisible()
    await check(page)
    await page.keyboard.press('Escape')
    await expect(editDialog).toBeHidden()
  })

  await test.step('the share view', async () => {
    // A new context inherits the viewport, the base URL and the English language choice.
    const context = await browser.newContext()
    await context.route(/^https?:\/\/(?!127\.0\.0\.1)/, (r) => r.abort())
    const visitor = await context.newPage()
    await visitor.goto(`/share/${token}`)
    await expect(visitor.getByText('Read-only view')).toBeVisible()
    await check(visitor)
    await visitor.getByRole('link', { name: `Start with Anna ${surname}` }).click()
    await expect(visitor.getByRole('heading', { level: 1, name: `Anna ${surname}` })).toBeVisible()
    await check(visitor)
    await visitor.getByRole('link', { name: 'Show in the tree' }).click()
    await expect(visitor.getByRole('group', { name: 'Family tree chart' })).toBeVisible()
    await check(visitor)
    await context.close()
  })
})
