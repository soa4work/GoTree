import { readFileSync } from 'node:fs'
import { expect, test } from './base'
import { expectAccessible, login, unique } from './helpers'

test('dark theme is remembered and keeps contrast on the main pages', { tag: '@mobile' }, async ({ page }) => {
  await login(page)
  const surname = unique('Dark', page)
  const anna = await (await page.request.post('/api/persons', { data: { givenNames: 'Anna', surname } })).json()

  await page.getByLabel('Theme').selectOption('dark')
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.reload()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await expect(page.getByLabel('Theme')).toHaveValue('dark')

  for (const path of ['/', '/people', `/people/${anna.id}`, `/tree?root=${anna.id}`, '/sources', '/quality', '/relationship', '/import-export']) {
    await page.goto(path)
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expectAccessible(page)
  }

  await page.getByLabel('Theme').selectOption('light')
  await expect(page.locator('html')).not.toHaveClass(/dark/)
})

test('download a backup', async ({ page }) => {
  await login(page)
  await page.goto('/import-export')
  const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('link', { name: 'Download backup' }).click()])
  expect(download.suggestedFilename()).toMatch(/^gotree-backup-\d{8}-\d{4}\.zip$/)
  // A zip starts with PK\3\4; the content is tested in Go.
  const zip = readFileSync((await download.path())!)
  expect(zip.subarray(0, 4).toString('latin1')).toBe('PK\x03\x04')
})
