import { existsSync } from 'node:fs'
import { defineConfig, devices } from '@playwright/test'

// The tests use the system's Chrome or Chromium instead of a downloaded
// browser: CI runners ship Chrome, developer machines have one anyway.
const candidates = [
  process.env.CHROME_PATH,
  '/usr/bin/google-chrome',
  '/usr/bin/google-chrome-stable',
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/opt/google/chrome/chrome',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
]
const executablePath = candidates.find((p) => p && existsSync(p))
if (!executablePath) throw new Error('No Chrome or Chromium found; set CHROME_PATH')

const port = Number(process.env.E2E_PORT ?? 18095)

export default defineConfig({
  testDir: './e2e',
  // One server and database for all tests, so they run one after another.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    launchOptions: { executablePath },
    trace: 'retain-on-failure',
    // The app defaults to Russian; the specs read the English source strings, so
    // every context starts with the hidden 'en' choice saved. Contexts made by
    // hand (browser.newContext) must call forceEnglish() from e2e/helpers.ts.
    // e2e/i18n.spec.ts switches to Russian and Kazakh on purpose.
    storageState: {
      cookies: [],
      origins: [{ origin: `http://127.0.0.1:${port}`, localStorage: [{ name: 'gotree-lang', value: 'en' }] }],
    },
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 } } },
    { name: 'mobile', use: { ...devices['Pixel 7'], viewport: { width: 390, height: 844 } } },
  ],
  webServer: {
    // A fresh data directory per run; the admin comes from the environment.
    command: `sh -c 'rm -rf .e2e-data && exec ../gotree -data-dir .e2e-data -listen 127.0.0.1:${port}'`,
    url: `http://127.0.0.1:${port}/api/health`,
    env: { GOTREE_ADMIN_USER: 'admin', GOTREE_ADMIN_PASSWORD: 'e2e correct horse' },
    reuseExistingServer: false,
    stdout: 'ignore',
    stderr: 'pipe',
  },
})
