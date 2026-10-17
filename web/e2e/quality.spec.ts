import { expect, test } from '@playwright/test'
import { expectAccessible, login, openSection, unique } from './helpers'

test('find contradictions, tidy dates and name a relationship', async ({ page }) => {
  await login(page)
  const surname = unique('Quality', page)
  const api = page.request

  const anna = await (await api.post('/api/persons', { data: { givenNames: 'Anna', surname, sex: 'F' } })).json()
  const sister = await (
    await api.post(`/api/persons/${anna.id}/relatives`, {
      data: { relation: 'sibling', person: { givenNames: 'Berta', surname, sex: 'F' } },
    })
  ).json()
  const nephew = await (
    await api.post(`/api/persons/${sister.person.id}/relatives`, {
      data: { relation: 'child', person: { givenNames: 'Carl', surname, sex: 'M' } },
    })
  ).json()
  await api.post('/api/events', { data: { personId: anna.id, type: 'BIRT', date: '1900' } })
  await api.post('/api/events', { data: { personId: anna.id, type: 'DEAT', date: '1890' } })
  await api.post('/api/events', { data: { personId: anna.id, type: 'RESI', date: 'ca. 1895' } })

  // The person page shows its own problems.
  await page.goto(`/people/${anna.id}`)
  const problems = page.getByRole('region', { name: 'Possible problems' })
  await expect(problems.getByText(`Anna ${surname} was born after they died.`, { exact: true })).toBeVisible()
  await expectAccessible(page)

  // The data quality page offers the loose date in GEDCOM form; only this
  // test's row is rewritten.
  await page.goto('/')
  await openSection(page, 'Data quality')
  await expect(page.getByRole('heading', { level: 1, name: 'Data quality' })).toBeVisible()
  await expect(page.getByText(`Anna ${surname} was born after they died.`, { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Select none' }).click()
  const unreadable = page.getByText(`Anna ${surname}’s Residence has a date GoTree cannot read.`, { exact: true })
  await expect(unreadable).toBeVisible()
  const row = page.getByRole('listitem').filter({ hasText: `Anna ${surname}` }).filter({ hasText: 'ca. 1895' })
  await row.getByRole('checkbox', { name: 'ca. 1895 becomes ABT 1895' }).check()
  await expectAccessible(page)
  await page.getByRole('button', { name: 'Rewrite 1 date' }).click()
  await expect(page.getByText('1 date rewritten.')).toBeVisible()
  await expect(row).toBeHidden()
  await expect(unreadable).toBeHidden()

  // From Carl's page to the calculator, then pick Anna.
  await page.goto(`/people/${nephew.person.id}`)
  await page.getByRole('link', { name: 'Relationship to…' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Relationships' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Second person' }).fill(`Anna ${surname}`)
  await page.getByRole('button', { name: new RegExp(`^Anna ${surname}`) }).click()
  await expect(page.getByText(`Anna ${surname} is Carl ${surname}’s aunt.`)).toBeVisible()
  const path = page.getByRole('region', { name: 'How they are connected' })
  await expect(path.getByRole('listitem')).toHaveCount(4)
  await expect(path.getByText('unknown parents', { exact: true })).toBeVisible()
  await expectAccessible(page)

  await page.getByRole('button', { name: /Swap/ }).click()
  await expect(page.getByText(`Carl ${surname} is Anna ${surname}’s nephew.`)).toBeVisible()
})
