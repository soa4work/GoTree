import type { Page } from '@playwright/test'
import { expect, test } from './base'
import { expectAccessible, login, unique } from './helpers'

/** Builds Paul with parents, a sibling, a partner and a child through the API. */
async function family(page: Page) {
  const surname = unique('Tree', page)
  const api = page.request
  const create = async (givenNames: string, sex: string) =>
    (await (await api.post('/api/persons', { data: { givenNames, surname, sex } })).json()) as { id: number }
  const relative = async (id: number, relation: string, givenNames: string, sex: string) => {
    const res = await api.post(`/api/persons/${id}/relatives`, { data: { relation, person: { givenNames, surname, sex } } })
    expect(res.ok()).toBe(true)
    return ((await res.json()) as { person: { id: number } }).person.id
  }
  const paul = await create('Paul', 'M')
  const hans = await relative(paul.id, 'parent', 'Hans', 'M')
  await relative(paul.id, 'parent', 'Maria', 'F')
  await relative(paul.id, 'sibling', 'Lena', 'F')
  await relative(hans, 'parent', 'Karl', 'M')
  await relative(paul.id, 'partner', 'Eva', 'F')
  await relative(paul.id, 'child', 'Mia', 'F')
  return { surname, paul: paul.id, hans }
}

test('pedigree chart: keyboard navigation, selection and centering', async ({ page }) => {
  await login(page)
  const { surname, paul, hans } = await family(page)

  await page.goto(`/tree?root=${paul}&view=pedigree&gen=3`)
  const chart = page.getByRole('group', { name: 'Family tree chart' })
  const paulCard = chart.getByRole('button', { name: new RegExp(`^♂?Paul ${surname}`) })
  await expect(paulCard).toBeVisible()
  await expect(chart.getByRole('button', { name: new RegExp(`Karl ${surname}`) })).toBeVisible()
  await expect(page.getByText('4 people shown.')).toBeVisible()
  await expectAccessible(page)

  // One tab stop in the chart; arrow up goes to a parent.
  await paulCard.focus()
  await page.keyboard.press('ArrowUp')
  const focused = page.locator(':focus')
  await expect(focused).toHaveAttribute('data-tree-person', /.+/)
  await expect(focused).not.toHaveAttribute('data-tree-person', String(paul))
  await page.keyboard.press('Enter')
  const panel = page.getByRole('region', { name: /(Hans|Maria) / })
  await expect(panel).toBeVisible()

  // Centering on Hans shows his ancestors and moves the root.
  await page.goto(`/tree?root=${paul}&view=pedigree&gen=3&sel=${hans}`)
  await page.getByRole('region', { name: `Hans ${surname}` }).getByRole('button', { name: 'Center tree here' }).click()
  await expect(page).toHaveURL(new RegExp(`root=${hans}`))
  await expect(page.getByRole('heading', { level: 1, name: `Tree of Hans ${surname}` })).toBeVisible()
})

test('a remembered person that was deleted does not break the tree', async ({ page }) => {
  await login(page)
  const { surname, paul } = await family(page)

  await page.goto(`/tree?root=${paul}`)
  await expect(page.getByRole('heading', { level: 1, name: `Tree of Paul ${surname}` })).toBeVisible()
  await page.goto('/')
  expect((await page.request.delete(`/api/persons/${paul}`)).ok()).toBe(true)

  await page.goto('/tree')
  await expect(page.getByRole('heading', { level: 1, name: /^(Tree of |Tree$)/ })).toBeVisible()
  await expect(page.getByText('Could not load the data. Please try again.')).toHaveCount(0)
  // The deleted person is not shown; the tree starts from another one.
  await expect(page.getByRole('heading', { level: 1, name: `Tree of Paul ${surname}` })).toHaveCount(0)
})

test('family group and list view', async ({ page }) => {
  await login(page)
  const { surname, paul } = await family(page)

  await page.goto(`/tree?root=${paul}&view=family`)
  const chart = page.getByRole('group', { name: 'Family tree chart' })
  for (const name of ['Hans', 'Maria', 'Lena', 'Eva', 'Mia']) {
    await expect(chart.getByRole('button', { name: new RegExp(`${name} ${surname}`) })).toBeVisible()
  }
  await expect(chart.getByRole('button', { name: new RegExp(`Karl ${surname}`) })).toHaveCount(0)

  await page.getByText('List', { exact: true }).click()
  await expect(page).toHaveURL(/mode=list/)
  await expect(page.getByRole('region', { name: 'Siblings' }).getByRole('link', { name: `Lena ${surname}` })).toBeVisible()
  await expect(page.getByRole('list', { name: `Children of Paul ${surname}` }).getByRole('link', { name: `Mia ${surname}` })).toBeVisible()
  await expectAccessible(page)
})

test('add a relative from the chart', async ({ page }) => {
  await login(page)
  const { surname, paul } = await family(page)
  await page.goto(`/tree?root=${paul}&view=descendants&gen=2&sel=${paul}`)
  const panel = page.getByRole('region', { name: `Paul ${surname}` })
  await panel.getByRole('button', { name: '+ Child' }).click()
  const dialog = page.getByRole('dialog', { name: `Add a child of Paul ${surname}` })
  await dialog.getByLabel('Given names').fill('Tom')
  await dialog.getByRole('button', { name: /^Add/ }).click()
  await expect(dialog).toBeHidden()
  await expect(
    page.getByRole('group', { name: 'Family tree chart' }).getByRole('button', { name: new RegExp(`Tom ${surname}`) }),
  ).toBeVisible()
})

test('phone: controls are collapsed and a tapped person opens a sheet', { tag: '@mobile-only' }, async ({ page }) => {
  await login(page)
  const { surname, paul } = await family(page)
  await page.goto(`/tree?root=${paul}&view=pedigree&gen=3`)

  const options = page.getByRole('button', { name: /^Options/ })
  await expect(options).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByLabel('Generations')).toBeHidden()
  const chart = page.getByRole('group', { name: 'Family tree chart' })
  await expect(chart).toBeVisible()
  expect((await chart.boundingBox())!.y).toBeLessThan(400)
  await options.click()
  await expect(page.getByLabel('Generations')).toBeVisible()
  await options.click()

  await chart.getByRole('button', { name: new RegExp(`Paul ${surname}`) }).tap()
  const sheet = page.getByRole('region', { name: `Paul ${surname}` })
  await expect(sheet).toBeVisible()
  await expect(sheet.getByRole('button', { name: '+ Parent' })).toBeInViewport()
  await expect(sheet).toBeFocused()
  // The selected card moves up so the sheet does not cover it.
  await expect
    .poll(async () => {
      const card = (await chart.getByRole('button', { name: new RegExp(`Paul ${surname}`) }).boundingBox())!
      return card.y + card.height <= (await sheet.boundingBox())!.y + 1
    })
    .toBe(true)
  await expectAccessible(page)

  await page.keyboard.press('Escape')
  await expect(sheet).toBeHidden()
  await expect(page).not.toHaveURL(/sel=/)
})
