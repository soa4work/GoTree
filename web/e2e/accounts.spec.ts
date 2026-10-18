import { expect, test } from '@playwright/test'
import { expectAccessible, login, openSection, unique } from './helpers'

// All specs share one database and one admin password, so this never resets the
// admin's password, changes the admin's role or deletes the admin: it creates an
// account of its own and deletes only that one again.
test('an administrator creates an account and deletes it again', async ({ page }) => {
  await login(page)
  await openSection(page, 'Accounts')
  await expect(page.getByRole('heading', { level: 1, name: 'Accounts' })).toBeVisible()

  // The own row can be edited, but not reset or deleted.
  const own = page.getByRole('listitem').filter({ hasText: '@admin' })
  await expect(own.getByRole('button', { name: /^Edit\s*: admin$/ })).toBeVisible()
  await expect(own.getByRole('button', { name: /^Reset password/ })).toHaveCount(0)
  await expect(own.getByRole('button', { name: /^Delete/ })).toHaveCount(0)

  const username = unique('e2euser', page)
  await page.getByRole('button', { name: '+ New account' }).click()
  const dialog = page.getByRole('dialog', { name: 'New account' })
  await dialog.getByLabel('Username').fill(username)
  await dialog.getByLabel('Display name').fill('E2E Guest')
  await dialog.getByLabel('New password').fill('e2e other password')
  await dialog.getByLabel('Repeat password').fill('e2e other password')
  await dialog.getByRole('button', { name: /^Create account/ }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText('The account E2E Guest was created.')).toBeVisible()

  const row = page.getByRole('listitem').filter({ hasText: `@${username}` })
  await expect(row).toBeVisible()
  await expect(row).toContainText('Not in any tree yet')
  await expect(row).toContainText('Has not signed in yet')
  await expectAccessible(page)

  await row.getByRole('button', { name: /^Delete/ }).click()
  const confirm = page.getByRole('dialog', { name: 'Delete the account E2E Guest?' })
  await confirm.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(confirm).toBeHidden()
  await expect(page.getByRole('listitem').filter({ hasText: `@${username}` })).toHaveCount(0)
})
