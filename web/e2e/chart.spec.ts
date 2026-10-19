import { readFileSync } from 'node:fs'
import { expect, test } from './base'
import { expectAccessible, login, unique } from './helpers'

test('print a wall chart and download it as SVG', async ({ page }) => {
  await login(page)
  const surname = unique('Chart', page)
  const paul = await (await page.request.post('/api/persons', { data: { givenNames: 'Paul', surname, sex: 'M' } })).json()
  await page.request.post(`/api/persons/${paul.id}/relatives`, {
    data: { relation: 'parent', person: { givenNames: 'Hans', surname, sex: 'M' } },
  })

  await page.goto(`/tree?root=${paul.id}`)
  await page.getByRole('link', { name: 'Print as wall chart' }).click()
  const chart = page.getByRole('img', { name: `Ancestors of Paul ${surname}: chart of 2 people` })
  await expect(chart).toBeVisible()
  await page.getByLabel('Paper').selectOption('A2')
  await page.getByLabel('Orientation').selectOption('portrait')
  await expect(chart).toHaveAttribute('width', '420mm')
  await expectAccessible(page)

  // In print only the chart remains, on a page of the chosen size.
  await page.emulateMedia({ media: 'print' })
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeHidden()
  await expect(page.getByLabel('Paper')).toBeHidden()
  await expect(chart).toBeVisible()
  const pageRules = (await page.locator('style').allTextContents()).join('\n')
  expect(pageRules).toContain('@page { size: 420mm 594mm; margin: 0 }')
  await page.emulateMedia({ media: 'screen' })

  const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Download SVG' }).click()])
  expect(download.suggestedFilename()).toBe(`Ancestors of Paul ${surname}.svg`)
  const svg = readFileSync((await download.path())!, 'utf8')
  expect(svg).toContain('<svg')
  expect(svg).toContain(`<title>Ancestors of Paul ${surname}</title>`)
  // Given names and surname are on lines of their own.
  expect(svg).toContain('>♂ Hans</text>')
  expect(svg).toMatch(/>Chart[^<]*<\/text>/)
  expect(svg).toContain('width="420mm"')
})
