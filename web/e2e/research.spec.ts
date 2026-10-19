import { expect, test } from './base'
import { expectAccessible, login, unique } from './helpers'

test('plan research from a person and log a search that found nothing', async ({ page }) => {
  await login(page)
  const surname = unique('Research', page)
  const anna = await (await page.request.post('/api/persons', { data: { givenNames: 'Anna', surname } })).json()
  await page.request.post('/api/events', { data: { personId: anna.id, type: 'BIRT', date: '1900' } })
  await page.request.post('/api/events', { data: { personId: anna.id, type: 'DEAT', date: '1890' } })

  await page.goto(`/people/${anna.id}`)
  const research = page.getByRole('region', { name: 'Research' })
  const suggestion = research.getByRole('region', { name: 'Worth researching' }).getByRole('listitem').filter({ hasText: `Find a source for Anna ${surname}’s birth` })
  await suggestion.getByRole('button', { name: /Make a task/ }).click()
  await expect(suggestion).toBeHidden()
  const todo = research.getByRole('region', { name: 'To do' })
  await expect(
    todo.getByRole('button', { name: new RegExp(`Find a source for Anna ${surname}’s birth`) }),
  ).toBeVisible()

  // A finding becomes a task from the list of possible problems.
  const problems = page.getByRole('region', { name: 'Possible problems' })
  await problems.getByRole('button', { name: /Make a task/ }).click()
  await expect(problems.getByRole('link', { name: 'Task created' })).toBeVisible()
  await expect(todo.getByRole('listitem')).toHaveCount(2)
  await expectAccessible(page)

  await research.getByRole('button', { name: '+ Log a search' }).click()
  const dialog = page.getByRole('dialog', { name: 'Log a search' })
  await dialog.getByLabel(/^What you searched for/).fill('Baptisms 1898–1902')
  await dialog.getByLabel(/^Where/).fill('Stadtarchiv Leipzig')
  await dialog.getByLabel('Task (optional)').selectOption({ label: `Find a source for Anna ${surname}’s birth` })
  await expect(dialog.getByRole('button', { name: `Remove Anna ${surname}` })).toBeVisible()
  await expectAccessible(page)
  await dialog.getByRole('button', { name: /^Save/ }).click()
  await expect(dialog).toBeHidden()
  await expect(research.getByRole('region', { name: 'Research log' }).getByText('Baptisms 1898–1902')).toBeVisible()

  await page.goto('/research')
  const task = page
    .getByRole('region', { name: 'To do' })
    .getByRole('button', { name: new RegExp(`Find a source for Anna ${surname}’s birth`) })
  await expect(task).toContainText('1 search')
  await expectAccessible(page)
  await task.click()
  const edit = page.getByRole('dialog', { name: 'Edit task' })
  await edit.getByLabel('Status').selectOption('done')
  await edit.getByRole('button', { name: /^Save/ }).click()
  await expect(task).toBeHidden()
  await page.getByLabel('Show').selectOption('done')
  await expect(
    page.getByRole('region', { name: 'To do' }).getByText(`Find a source for Anna ${surname}’s birth`),
  ).toBeVisible()
})
