import { expect, test } from '@playwright/test'
import { expectAccessible, login, unique } from './helpers'

test('login page is accessible', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible()
  await expectAccessible(page)
})

test('build a small family by keyboard and mouse', async ({ page }) => {
  await login(page)
  const surname = unique('Weber', page)

  // New person with the n shortcut; the dialog focuses the first field.
  await page.goto('/people')
  await expect(page.locator('#people-status')).toBeVisible()
  await page.keyboard.press('n')
  const dialog = page.getByRole('dialog', { name: 'New person' })
  await expect(dialog.getByLabel('Given names')).toBeFocused()
  await page.keyboard.type('Paul')
  await dialog.getByLabel('Surname').fill(surname)
  await dialog.getByLabel('Surname').press('Control+Enter')

  await expect(page.getByRole('heading', { level: 1, name: `Paul ${surname}` })).toBeVisible()
  await expectAccessible(page)

  // A birth with a German-style date and a new nested place.
  await page.getByRole('button', { name: /^\+ Event/ }).click()
  const eventDialog = page.getByRole('dialog', { name: 'New life event' })
  await eventDialog.getByLabel('Date').fill('12.3.1890')
  await expect(eventDialog.getByText('Understood as 12 March 1890')).toBeVisible()
  const place = `Leipzig${surname}, Saxony, Germany`
  await eventDialog.getByRole('searchbox', { name: 'Place' }).fill(place)
  await eventDialog.getByRole('button', { name: `Create “${place}”` }).click()
  await expect(eventDialog.getByText(place)).toBeVisible()
  await expectAccessible(page)
  await eventDialog.getByRole('button', { name: /^Save/ }).click()
  await expect(eventDialog).toBeHidden()
  await expect(page.getByText(`12 March 1890 · ${place}`)).toBeVisible()

  // A parent before anything else is known, then a sibling.
  await page.getByRole('button', { name: /^\+ Parent/ }).click()
  const parentDialog = page.getByRole('dialog', { name: `Add a parent of Paul ${surname}` })
  await parentDialog.getByLabel('Given names').fill('Hans')
  await parentDialog.getByLabel('Surname').fill(surname)
  await parentDialog.getByRole('button', { name: /^Add/ }).click()
  const parents = page.getByRole('region', { name: 'Parents and siblings' })
  await expect(parents.getByRole('link', { name: `Hans ${surname}` })).toBeVisible()
  await expect(parents.getByText('Unknown parent')).toBeVisible()

  await page.keyboard.press('b')
  const siblingDialog = page.getByRole('dialog', { name: `Add a sibling of Paul ${surname}` })
  await expect(siblingDialog.getByLabel('Surname')).toHaveValue(surname)
  await siblingDialog.getByLabel('Given names').fill('Lena')
  await siblingDialog.getByRole('button', { name: /^Add/ }).click()
  await expect(parents.getByRole('link', { name: `Lena ${surname}` })).toBeVisible()

  // Esc closes a dialog and focus returns to the button that opened it.
  const editButton = page.getByRole('button', { name: /^Edit/ }).first()
  await editButton.click()
  await expect(page.getByRole('dialog', { name: 'Edit person' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Edit person' })).toBeHidden()
  await expect(editButton).toBeFocused()

  // The father's page shows both children.
  await parents.getByRole('link', { name: `Hans ${surname}` }).click()
  const children = page.getByRole('region', { name: 'Partners and children' })
  await expect(children.getByRole('link', { name: `Paul ${surname}` })).toBeVisible()
  await expect(children.getByRole('link', { name: `Lena ${surname}` })).toBeVisible()

  // Search ignores case and finds all three.
  await page.goto('/people')
  await page.getByRole('searchbox', { name: 'Search people' }).fill(surname.toLowerCase())
  await expect(page.locator('#people-status')).toHaveText('3 people found')
  await expectAccessible(page)
})

test('delete a person after confirming', async ({ page }) => {
  await login(page)
  const name = unique('Gone', page)
  await page.goto('/people?new=1')
  await page.getByRole('dialog', { name: 'New person' }).getByLabel('Given names').fill(name)
  await page.getByRole('dialog', { name: 'New person' }).getByRole('button', { name: /^Save/ }).click()
  await expect(page.getByRole('heading', { level: 1, name })).toBeVisible()

  await page.getByRole('button', { name: 'Delete person' }).click()
  const confirm = page.getByRole('dialog', { name: 'Delete person' })
  await expect(confirm.getByText(`Delete ${name} with all their events?`)).toBeVisible()
  await confirm.getByRole('button', { name: 'Delete' }).click()
  await expect(page).toHaveURL(/\/people$/)
  await page.getByRole('searchbox', { name: 'Search people' }).fill(name)
  await expect(page.locator('#people-status')).toHaveText('0 people found')
})
