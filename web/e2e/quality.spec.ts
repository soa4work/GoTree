import { expect, test } from '@playwright/test'
import { expectAccessible, login, openSection, unique } from './helpers'

test('find contradictions, unreadable dates and name a relationship', async ({ page }) => {
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
  await api.post('/api/events', { data: { personId: anna.id, type: 'RESI', date: 'sometime in 1895' } })

  // The person page shows its own problems.
  await page.goto(`/people/${anna.id}`)
  const problems = page.getByRole('region', { name: 'Possible problems' })
  await expect(problems.getByText(`Anna ${surname} was born after they died.`, { exact: true })).toBeVisible()
  await expectAccessible(page)

  // The data quality page lists the unreadable date.
  await page.goto('/')
  await openSection(page, 'Data quality')
  await expect(page.getByRole('heading', { level: 1, name: 'Data quality' })).toBeVisible()
  await expect(page.getByText(`Anna ${surname} was born after they died.`, { exact: true })).toBeVisible()
  const unreadable = page.getByText(`Anna ${surname}’s Residence has a date GoTree cannot read.`, { exact: true })
  await expect(unreadable).toBeVisible()
  // Dates typed as "ca. 1895" are saved as GEDCOM, so nothing is left to tidy here;
  // rewriting older text is covered by the Go and component tests.
  await expectAccessible(page)

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
