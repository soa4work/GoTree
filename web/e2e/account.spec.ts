import { expect, test } from './base'
import { expectAccessible, login, openSection } from './helpers'

// Changing the password or signing out everywhere would break the other specs,
// which share a database with the other tests of their worker and the password
// from base.ts, so this only looks.
test('the account page lists this device', async ({ page }) => {
  await login(page)
  await openSection(page, 'Account')
  await expect(page.getByRole('heading', { level: 1, name: 'Account' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Signed-in devices' })).toBeVisible()
  await expect(page.getByText('This device')).toBeVisible()
  await expectAccessible(page)
})
