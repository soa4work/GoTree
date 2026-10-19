import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect, test } from './base'
import { expectAccessible, login } from './helpers'

const ancestry = readFileSync(fileURLToPath(new URL('./fixtures/ancestry-551.ged', import.meta.url)))

test('export a GEDCOM file and check it is complete', async ({ page }) => {
  await login(page)
  // A known state: the Ancestry fixture replaces whatever other tests left.
  const imported = await page.request.post('/api/import/gedcom?mode=replace', {
    multipart: { file: { name: 'tree.ged', mimeType: 'text/plain', buffer: ancestry } },
  })
  expect(imported.ok()).toBe(true)

  await page.goto('/import-export')
  const exportSection = page.getByRole('region', { name: 'Export a GEDCOM file' })
  await exportSection.getByLabel('GEDCOM 7.0').check()
  await exportSection.getByLabel('Living people with their name only').check()
  await expectAccessible(page)

  const [download] = await Promise.all([page.waitForEvent('download'), exportSection.getByRole('link', { name: 'Download' }).click()])
  expect(download.suggestedFilename()).toMatch(/\.ged$/)
  const text = readFileSync((await download.path())!, 'utf8')
  expect(text).toContain('2 VERS 7.0')
  expect(text).toContain('1 NAME Johann /Weber/')

  await exportSection.getByRole('button', { name: 'Check the export' }).click()
  await expect(exportSection.getByText('GEDCOM 7.0: everything came back. The export is complete.')).toBeVisible()
  await expect(exportSection.getByRole('table', { name: 'Records in the tree and after exporting and reading back' })).toContainText('People44')
  await expectAccessible(page)
})
