import { expect, test } from './base'
import { expectAccessible, login, openSection } from './helpers'

test('grouped navigation reaches every section', { tag: '@mobile' }, async ({ page }) => {
  await login(page)
  const nav = page.getByRole('navigation', { name: 'Main navigation' })

  if ((page.viewportSize()?.width ?? 0) < 768) {
    await expect(nav.getByRole('link')).toHaveCount(4)
    const more = nav.getByRole('button', { name: 'More' })
    await more.click()
    const sheet = page.getByRole('dialog', { name: 'More' })
    await expect(sheet.getByRole('heading', { name: 'Settings' })).toBeVisible()
    await expect(sheet.getByRole('list', { name: 'Records' }).getByRole('link')).toHaveCount(4)
    await expectAccessible(page)

    // Escape closes the sheet and returns focus to the button.
    await page.keyboard.press('Escape')
    await expect(sheet).toBeHidden()
    await expect(more).toBeFocused()
  } else {
    await expect(nav.getByRole('list', { name: 'Tree' }).getByRole('link')).toHaveText(['People', 'Tree', 'Wall chart', 'Relationships'])
    await expect(nav.getByRole('list', { name: 'Settings' }).getByRole('link')).toHaveText(['Sharing', 'Members', 'Import & export', 'Account', 'Accounts'])
  }

  await openSection(page, 'Sharing')
  await expect(page.getByRole('heading', { level: 1, name: 'Sharing' })).toBeVisible()

  await openSection(page, 'Wall chart')
  await expect(page.getByRole('heading', { level: 1, name: 'Wall chart' })).toBeVisible()
})
