import AxeBuilder from '@axe-core/playwright'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
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

export type Lang = 'ru' | 'kk'

/** Switches an open page's context to Russian or Kazakh; it applies from the next navigation. */
export async function useLanguage(page: Page, lang: Lang) {
  await page.evaluate((l) => localStorage.setItem('gotree-lang', l), lang)
}

/** Fails when the page is wider than the screen, naming the first elements that stick out. */
export async function expectNoHorizontalScroll(page: Page) {
  const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  try {
    await expect.poll(overflow).toBeLessThanOrEqual(0)
  } catch (e) {
    const culprits = await page.evaluate(() =>
      [...document.querySelectorAll('body *')]
        .filter((el) => el.getBoundingClientRect().right > window.innerWidth + 1)
        .slice(0, 5)
        .map((el) => `${el.tagName.toLowerCase()}.${String(el.className).slice(0, 60)} "${(el.textContent ?? '').trim().slice(0, 40)}"`),
    )
    throw new Error(`${page.url()} is wider than the screen: ${culprits.join(' | ')}\n${String(e)}`)
  }
}

type Catalog = { key: string; re: RegExp }[]
const catalogs = new Map<Lang, Catalog>()

function flatten(o: unknown, prefix = '', out: Record<string, string> = {}) {
  if (typeof o === 'string') out[prefix] = o
  else if (o && typeof o === 'object') for (const [k, v] of Object.entries(o)) flatten(v, prefix ? `${prefix}.${k}` : k, out)
  return out
}

function readLocale(name: string) {
  return flatten(JSON.parse(readFileSync(fileURLToPath(new URL(`../src/locales/${name}.json`, import.meta.url)), 'utf8')))
}

/** Locale JSON as flat key/value pairs, so specs need not hard-code translated names. */
export function localeText(lang: Lang, key: string): string {
  const text = readLocale(lang)[key]
  if (text === undefined) throw new Error(`no ${lang} text for ${key}`)
  return text
}

/** The English texts that a translation left unchanged (and that contain real words). */
function catalog(lang: Lang): Catalog {
  let c = catalogs.get(lang)
  if (c) return c
  const en = readLocale('en')
  const own = readLocale(lang)
  c = []
  for (const [key, text] of Object.entries(en)) {
    if (own[key] !== undefined && own[key] !== text) continue
    const bare = text.replace(/\{\{[^}]*\}\}/g, '').replace(/<\/?\w+>/g, '')
    if ((bare.match(/[A-Za-z]/g) ?? []).length < 4) continue
    const source = text
      .replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      .replace(/\\\{\\\{[^}]*\\\}\\\}/g, '.+?')
      .replace(/<\/?\w+>/g, '')
      .replace(/\s+/g, '\\s+')
    c.push({ key, re: new RegExp(`^${source}$`, 'i') })
  }
  catalogs.set(lang, c)
  return c
}

// Words the third-party libraries and the server used to show in English.
const leftovers: RegExp[] = [
  /zoom in/i, /zoom out/i, /fit view/i, /toggle interactivity/i, /control panel/i, /mini map/i,
  /press enter or space/i, /a javascript library for interactive maps/i, /failed to fetch/i,
  /networkerror/i, /load failed/i, /bad gateway/i, /service unavailable/i, /gateway timeout/i,
  /internal server error/i, /\bunknown\b/i, /\bfamily tree\b/i, /does not exist/i, /\bskipped\b/i, /\bline \d+/i,
]

// Proper names and technical tokens that stay as they are in every language.
const allowed: RegExp[] = [
  /GoTree|GEDCOM|OpenStreetMap|Nominatim|Leaflet|React Flow|Ancestry|MyHeritage|FamilySearch|webtrees|Gramps|RootsMagic/g,
  /CC BY 4\.0|UTF-8|PDF|SVG|\bA[0-4]\b|Ctrl|Enter|Esc/g,
  /\b[A-Z_]{3,5}(\.[A-Z_]{3,5})*\b/g,
  /\.[a-z]{2,4}\b/g,
  /lang=\w+/g,
  /https?:\/\/\S+/g,
]

/**
 * Fails when text in English is left on a page that is shown in ru or kk: a UI string whose
 * translation equals the English source, a known third-party or server phrase, or a raw i18n key.
 * User data is not matched by "any Latin word" on purpose: other specs leave Latin names behind.
 */
export async function expectNoEnglish(page: Page, lang: Lang) {
  const texts = await page.evaluate(() => {
    const skip = 'script, style, .react-flow__attribution, .leaflet-control-attribution'
    const out = new Set<string>()
    const add = (s: string | null | undefined) => {
      const t = (s ?? '').replace(/\s+/g, ' ').trim()
      if (t) out.add(t)
    }
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const el = n.parentElement
      if (!el || el.closest(skip) || !el.checkVisibility()) continue
      add(n.textContent)
    }
    for (const el of document.body.querySelectorAll('*')) {
      if (el.closest(skip) || !el.checkVisibility()) continue
      for (const a of ['aria-label', 'title', 'placeholder', 'alt', 'aria-description', 'aria-roledescription']) add(el.getAttribute(a))
    }
    add(document.title)
    return [...out]
  })
  const cat = catalog(lang)
  const offending: string[] = []
  for (const original of texts) {
    if (/^[a-z][A-Za-z]+(\.[A-Za-z_]+)+$/.test(original)) {
      offending.push(`"${original}" (raw key)`)
      continue
    }
    let text = original
    for (const re of allowed) text = text.replace(re, ' ')
    if (!/[A-Za-z]/.test(text)) continue
    const hit = cat.find((c) => c.re.test(original))
    if (hit) offending.push(`"${original}" (en key ${hit.key})`)
    else if (leftovers.some((re) => re.test(text))) offending.push(`"${original}" (known English phrase)`)
  }
  expect(offending, `English left in ${lang} on ${page.url()}`).toEqual([])
}
