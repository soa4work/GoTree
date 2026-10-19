import { expect, test } from './base'
import { expectAccessible, login, openSection } from './helpers'

// Switching or creating trees, adding members or leaving would move the shared admin
// account (and every later login) into another tree, since the tests of a worker
// share one database, so this only looks.
test('the members page lists the owner', async ({ page }) => {
  await login(page)
  await openSection(page, 'Members')
  await expect(page.getByRole('heading', { level: 1, name: 'Members' })).toBeVisible()
  const admin = page.getByRole('listitem').filter({ hasText: '@admin' })
  await expect(admin).toBeVisible()
  await expect(admin.getByRole('combobox')).toHaveValue('owner')
  await expect(admin.getByRole('combobox')).toContainText('Owner')
  await expectAccessible(page)
})
