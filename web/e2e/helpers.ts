import AxeBuilder from '@axe-core/playwright'
import { expect, type BrowserContext, type Page } from '@playwright/test'

/** Saves the hidden 'en' language choice for a context made by hand; contexts from the config already have it. */
export async function forceEnglish(context: BrowserContext) {
  await context.addInitScript(() => localStorage.setItem('gotree-lang', 'en'))
}

export async function login(page: Page) {
  await page.goto('/')
  await page.getByLabel('Username').fill('admin')
  await page.getByLabel('Password').fill('e2e correct horse')
  await page.getByRole('button', { name: 'Log in' }).click()
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()
}

/** Fails with the list of WCAG 2.2 AA violations on the current page. */
export async function expectAccessible(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
  expect(results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`)).toEqual([])
}

/** A name that is unique per test run and project, since all share one database. */
export function unique(base: string, page: Page) {
  const viewport = page.viewportSize()?.width ?? 0
  return `${base}${viewport < 600 ? 'm' : 'd'}${Date.now().toString(36)}`
}

/** Opens a section from the navigation: the sidebar on desktop, the bar or its More menu on a phone. */
export async function openSection(page: Page, name: string) {
  const nav = page.getByRole('navigation', { name: 'Main navigation' })
  const link = nav.getByRole('link', { name, exact: true })
  if ((page.viewportSize()?.width ?? 0) >= 768 || (await link.count()) > 0) {
    await link.click()
    return
  }
  await nav.getByRole('button', { name: 'More' }).click()
  await page.getByRole('navigation', { name: 'More sections' }).getByRole('link', { name, exact: true }).click()
}
