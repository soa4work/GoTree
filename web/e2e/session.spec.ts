import { expect, test } from './base'
import { login, unique } from './helpers'

test('an expired session during a save brings up the login screen', async ({ page, context }) => {
  await login(page)
  const surname = unique('Expired', page)
  const p = (await (await page.request.post('/api/persons', { data: { givenNames: 'Erna', surname } })).json()) as { id: number }
  await page.goto(`/people/${p.id}`)
  await expect(page.getByRole('heading', { level: 1, name: `Erna ${surname}` })).toBeVisible()

  await page.getByRole('button', { name: /^\+ Event/ }).click()
  const dialog = page.getByRole('dialog', { name: 'New life event' })
  await dialog.getByLabel('Date').fill('1890')
  await context.clearCookies()
  await dialog.getByRole('button', { name: /^Save/ }).click()

  await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible()
})
