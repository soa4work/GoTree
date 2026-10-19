import { expect, test } from './base'
import { expectAccessible, expectNoEnglish, expectNoHorizontalScroll, localeText, login, useLanguage } from './helpers'

// The only spec that leaves English on purpose: it walks the pages in Russian (the default)
// and Kazakh, at desktop and phone width, as issue #61 asks.
for (const lang of ['ru', 'kk'] as const) {
  test(`every main page is in ${lang}, fits the screen and passes axe`, async ({ page }) => {
    test.slow()
    // Map tiles and Nominatim are outside the test; aborting them keeps networkidle reachable.
    await page.route(/^https?:\/\/(?!127\.0\.0\.1)/, (r) => r.abort())
    await login(page) // English labels; the context starts with gotree-lang=en
    // Cyrillic names, so the data cannot look like leftover English.
    const res = await page.request.post('/api/persons', {
      data: { givenNames: 'Айгерим', surname: 'Тестова', sex: 'F', birth: { date: '12 MAR 1890' }, death: { date: '1950' } },
    })
    expect(res.ok()).toBe(true)
    const person = (await res.json()) as { id: number }
    await useLanguage(page, lang)

    const paths = [
      '/', '/people', `/people/${person.id}`, `/tree?root=${person.id}`, '/chart', '/relationship', '/sources',
      '/media', '/transcribe', '/transcribe/new', '/heirlooms', '/map', '/stats', '/research', '/quality',
      '/sharing', '/import-export', '/account', '/no-such-page',
    ]
    for (const path of paths) {
      await test.step(path, async () => {
        await page.goto(path)
        await expect(page.locator('html')).toHaveAttribute('lang', lang)
        await expect(page.getByRole('heading', { level: 1 }).first()).toBeVisible()
        await page.waitForLoadState('networkidle')
        await expectNoHorizontalScroll(page)
        await expectNoEnglish(page, lang)
        await expectAccessible(page)
      })
    }

    // The phone bar's More sheet (navigation.spec does the same in English).
    if ((page.viewportSize()?.width ?? 0) < 768) {
      await test.step('more sheet', async () => {
        await page.goto('/')
        const nav = page.getByRole('navigation', { name: localeText(lang, 'app.mainNavigation') })
        await nav.getByRole('button', { name: localeText(lang, 'nav.more') }).click()
        await expect(page.getByRole('dialog')).toBeVisible()
        await expectNoHorizontalScroll(page)
        await expectNoEnglish(page, lang)
        await expectAccessible(page)
      })
    }

    // The person line agrees with the sex (issue point 1.10).
    if (lang === 'ru') {
      await page.goto(`/people/${person.id}`)
      await expect(page.getByText(/умерла/)).toBeVisible()
    }
  })

  test(`the login page is in ${lang}`, async ({ browser, page }) => {
    const context = await browser.newContext({ viewport: page.viewportSize() ?? undefined })
    await context.addInitScript((l) => localStorage.setItem('gotree-lang', l), lang)
    const visitor = await context.newPage()
    await visitor.goto('/')
    await expect(visitor.getByRole('heading', { level: 1 })).toBeVisible()
    await expectNoHorizontalScroll(visitor)
    await expectNoEnglish(visitor, lang)
    await expectAccessible(visitor)
    await context.close()
  })
}
