import { expect, test } from '@playwright/test'
import { expectAccessible, forceEnglish, login, unique } from './helpers'

const months = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']

test('share a branch read-only and withdraw the link', async ({ page, browser }) => {
  await login(page)
  const surname = unique('Share', page)
  const api = page.request
  const hans = await (await api.post('/api/persons', { data: { givenNames: 'Hans', surname, sex: 'M' } })).json()
  const today = new Date()
  const birthday = `${today.getDate()} ${months[today.getMonth()]} 1880`
  await api.post('/api/events', { data: { personId: hans.id, type: 'BIRT', date: birthday } })
  await api.post('/api/events', { data: { personId: hans.id, type: 'DEAT', date: '1950' } })
  const lisa = await (
    await api.post(`/api/persons/${hans.id}/relatives`, {
      data: { relation: 'child', person: { givenNames: 'Lisa', surname, sex: 'F', notes: 'private note' } },
    })
  ).json()
  await api.post('/api/events', { data: { personId: lisa.person.id, type: 'BIRT', date: '1990' } })

  await page.goto('/sharing')
  await page.getByRole('button', { name: '+ New share link' }).click()
  const dialog = page.getByRole('dialog', { name: 'New share link' })
  await dialog.getByLabel(/^Name/).fill(`Cousins ${surname}`)
  await dialog.getByLabel('One branch').check()
  await dialog.getByRole('searchbox', { name: 'Whose branch' }).fill(`Hans ${surname}`)
  await dialog.getByRole('button', { name: new RegExp(`^Hans ${surname}`) }).click()
  await dialog.getByLabel('Show living people by name only').check()
  await expectAccessible(page)
  await dialog.getByRole('button', { name: /Create link/ }).click()
  const url = await page.getByLabel('Share link', { exact: true }).inputValue()
  expect(url).toMatch(/\/share\/[A-Za-z0-9_-]{40,}$/)
  await expectAccessible(page)

  // A relative without an account.
  const visitorContext = await browser.newContext({ viewport: page.viewportSize() ?? undefined })
  await forceEnglish(visitorContext)
  const visitor = await visitorContext.newPage()
  await visitor.goto(url)
  await expect(visitor.getByText('Read-only view')).toBeVisible()
  await expect(visitor.getByText('Living people are shown by name only.')).toBeVisible()
  const onThisDay = visitor.getByRole('region', { name: /^On this day/ })
  await expect(onThisDay.getByText(`Hans ${surname}`)).toBeVisible()
  await expect(onThisDay).toContainText('1880')
  await expectAccessible(visitor)

  await visitor.getByRole('link', { name: `Start with Hans ${surname}` }).click()
  await expect(visitor.getByRole('heading', { level: 1, name: `Hans ${surname}` })).toBeVisible()
  await expect(visitor.getByRole('button', { name: /Edit/ })).toHaveCount(0)
  await expectAccessible(visitor)
  await visitor.getByRole('link', { name: `Lisa ${surname}` }).click()
  await expect(visitor.getByRole('heading', { level: 1, name: `Lisa ${surname}` })).toBeVisible()
  await expect(visitor.getByText('Details of living people are private.')).toBeVisible()
  await expect(visitor.getByText('private note')).toHaveCount(0)
  await expect(visitor.getByText('1990')).toHaveCount(0)

  await visitor.getByRole('link', { name: 'Show in the tree' }).click()
  await expect(visitor.getByRole('group', { name: 'Family tree chart' })).toBeVisible()
  await expectAccessible(visitor)

  await page.getByRole('button', { name: 'Done' }).click()
  const row = page.getByRole('listitem').filter({ hasText: `Cousins ${surname}` })
  await row.getByRole('button', { name: /Withdraw/ }).click()
  await page.getByRole('dialog', { name: 'Withdraw share link' }).getByRole('button', { name: 'Withdraw' }).click()
  await expect(row.getByText('withdrawn', { exact: true })).toBeVisible()

  await visitor.goto(url)
  await expect(visitor.getByRole('heading', { name: 'This link does not work' })).toBeVisible()
  await visitor.context().close()
})

test('subscribe to the birthday calendar', async ({ page, playwright }) => {
  await login(page)
  const surname = unique('Cal', page)
  const anna = await (await page.request.post('/api/persons', { data: { givenNames: 'Anna', surname } })).json()
  await page.request.post('/api/events', { data: { personId: anna.id, type: 'BIRT', date: '12 MAR 1850' } })
  await page.request.post('/api/events', { data: { personId: anna.id, type: 'DEAT', date: '1920' } })

  await page.goto('/sharing')
  await page.getByRole('button', { name: '+ New calendar address' }).click()
  const dialog = page.getByRole('dialog', { name: 'New calendar address' })
  await dialog.getByLabel(/^Name/).fill(`Phone ${surname}`)
  await dialog.getByRole('button', { name: /Create address/ }).click()
  const url = await page.getByLabel('Calendar address', { exact: true }).inputValue()
  expect(url).toMatch(/\/ical\/[A-Za-z0-9_-]{40,}\.ics\?lang=en$/)
  await expectAccessible(page)

  // A calendar app has no cookies.
  const app = await playwright.request.newContext()
  const feed = await app.get(url)
  expect(feed.headers()['content-type']).toContain('text/calendar')
  expect(await feed.text()).toContain(`SUMMARY:Birthday: Anna ${surname} (born 1850)`)
  await app.dispose()
})
