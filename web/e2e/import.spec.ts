import { fileURLToPath } from 'node:url'
import { expect, test } from '@playwright/test'
import { expectAccessible, login } from './helpers'

const ancestry = fileURLToPath(new URL('./fixtures/ancestry-551.ged', import.meta.url))

test('import a GEDCOM file and read the report', async ({ page }) => {
  await login(page)
  // Other tests leave people behind, so the tree is replaced.
  await page.request.post('/api/persons', { data: { givenNames: 'Before import' } })
  await page.goto('/import-export')
  await expect(page.getByRole('heading', { level: 1, name: 'Import & export' })).toBeVisible()
  await expectAccessible(page)

  await page.getByLabel('GEDCOM file', { exact: true }).setInputFiles(ancestry)
  await page.getByRole('button', { name: 'Replace the tree' }).click()
  const confirm = page.getByRole('dialog', { name: 'Replace the tree?' })
  await confirm.getByRole('button', { name: 'Replace the tree' }).click()

  const report = page.getByRole('region', { name: 'Import finished' })
  await expect(report).toBeVisible()
  await expect(report).toContainText('From Ancestry.com Member Trees, GEDCOM 5.5.1 in UTF-8')
  await expect(report.getByText('People', { exact: true }).locator('..')).toContainText('4')
  await expect(report).toContainText('1 date is not in GEDCOM format')
  await report.getByText(/What happened to every GEDCOM tag/).click()
  await expect(report.getByRole('table', { name: 'Kept as is' })).toContainText('INDI._UID')
  await expectAccessible(page)

  await report.getByRole('link', { name: 'Go to the people' }).click()
  await page.getByRole('searchbox', { name: 'Search people' }).fill('johann')
  await page.getByRole('link', { name: /Johann Weber/ }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Johann Weber' })).toBeVisible()
  await expect(page.getByText('3 February 1855 · Leipzig, Sachsen, Deutschland')).toBeVisible()
  await expect(page.getByRole('list', { name: 'Sources for Birth' })).toContainText('Kirchenbuch St. Thomas, Taufen 1880-1900, S. 12, Nr. 34')
  // A GEDCOM date the importer could not read is shown as written.
  await expect(page.getByText('12 Dezember 1920')).toBeVisible()
})
