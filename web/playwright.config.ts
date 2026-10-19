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

/** E2E_WORKERS is a count or a percentage of the cores; anything else is ignored. */
function workersFrom(value: string | undefined): number | string | undefined {
  if (!value) return undefined
  if (/^\d+%$/.test(value)) return value
  const n = Number(value)
  return Number.isInteger(n) && n > 0 ? n : undefined
}

export default defineConfig({
  testDir: './e2e',
  // Each worker starts its own server and database (e2e/base.ts), so files run in parallel.
  // One Chrome renderer (axe is CPU-bound) plus a light Go server per worker. In CI the job
  // has a 4-vCPU runner to itself; locally half the cores leave room for the IDE and other gates.
  workers: workersFrom(process.env.E2E_WORKERS) ?? (process.env.CI ? 3 : '50%'),
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    launchOptions: { executablePath },
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 } } },
    { name: 'mobile', use: { ...devices['Pixel 7'], viewport: { width: 390, height: 844 } } },
  ],
})
