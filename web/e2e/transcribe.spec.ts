import { expect, test } from './base'
import { expectAccessible, login, unique } from './helpers'

test('transcribe a census household and cite each line', async ({ page }) => {
  await login(page)
  // Desktop and mobile share the database; fuzzy matching would pair
  // names that differ only in the unique suffix, so each run has its own.
  const mobile = (page.viewportSize()?.width ?? 0) < 600
  const surname = unique(mobile ? 'Krause' : 'Meyer', page)
  // In the tree already, spelled differently.
  const variant = mobile ? surname.replace('Krause', 'Kraus') : surname.replace('Meyer', 'Maier')
  const wife = mobile ? 'Greta' : 'Maria'
  const johann = await (
    await page.request.post('/api/persons', { data: { givenNames: 'Johann', surname: variant, sex: 'M' } })
  ).json()
  await page.request.post('/api/events', { data: { personId: johann.id, type: 'BIRT', date: '1861' } })
  const source = `Census 1900 ${surname}`

  await page.goto('/transcribe')
  await page.getByRole('link', { name: '+ Census household' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Transcribe a record' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Source' }).fill(source)
  await page.getByRole("button", { name: `Create source “${source}”` }).click()
  await page.getByLabel('Page or folio').fill('p. 4')
  await page.getByLabel('Date', { exact: true }).fill('1 DEC 1900')

  const first = page.getByRole('group', { name: 'Person 1' })
  await first.getByLabel('Line').fill('12')
  await first.getByLabel('Role').selectOption({ label: 'Head of household' })
  await first.getByLabel('Given names').fill('Johann')
  await first.getByLabel('Surname').fill(surname)
  await first.getByLabel('Age').fill('39')
  await first.getByLabel('Occupation').fill('Weber')

  await page.getByRole('button', { name: '+ Add a person' }).click()
  const second = page.getByRole('group', { name: 'Person 2' })
  await second.getByLabel('Line').fill('13')
  await second.getByLabel('Role').selectOption({ label: 'Spouse' })
  await second.getByLabel('Given names').fill(wife)
  await second.getByLabel('Surname').fill(surname)
  await second.getByLabel('Sex').fill('w')
  await second.getByLabel('Age').fill('35')
  await second.getByLabel('Birthplace').fill(`Halle${surname}, Sachsen`)
  await expectAccessible(page)

  await page.getByRole('button', { name: 'Find matches in the tree' }).click()
  await expect(first.getByRole('radio', { name: new RegExp(`Johann ${variant}`) })).toBeChecked()
  await expect(second.getByRole('radio', { name: 'A new person' })).toBeChecked()
  await expectAccessible(page)

  await page.getByRole('button', { name: 'Add to the tree' }).click()
  await expect(page.getByText('Added to the tree: the event, 2 further facts and 1 new person.')).toBeVisible()
  await page.getByRole('link', { name: `Open ${wife} ${surname}` }).click()

  await expect(page.getByRole('heading', { level: 1, name: `${wife} ${surname}` })).toBeVisible()
  const events = page.getByRole('region', { name: 'Life events' })
  await expect(events.getByText('between 1864 and 1865')).toBeVisible()
  await expect(events.getByText(`p. 4, line 13 (for the date)`)).toBeVisible()
  await expect(events.getByText('as spouse')).toBeVisible()
  await expectAccessible(page)

  await page.goto(`/people/${johann.id}`)
  await expect(page.getByRole('region', { name: 'Life events' }).getByText('Census', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Life events' }).getByText('Weber', { exact: true })).toBeVisible()

  await page.goto('/transcribe')
  await expect(
    page.getByRole('link', { name: new RegExp(`Census household without a title.*${source}.*applied`) }),
  ).toBeVisible()
})
