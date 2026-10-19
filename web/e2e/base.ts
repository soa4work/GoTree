import { spawn, type ChildProcess } from 'node:child_process'
import { existsSync, rmSync } from 'node:fs'
import { createServer } from 'node:net'
import { fileURLToPath } from 'node:url'
import { test as base, expect } from '@playwright/test'

/** The administrator every worker's server starts with; helpers.login uses it too. */
export const ADMIN = { user: 'admin', password: 'e2e correct horse' }

type Server = { url: string }

/** Asks the OS for a free port. Another process may take it before the server listens, so callers retry. */
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = createServer()
    s.once('error', reject)
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address() as { port: number }
      s.close(() => resolve(port))
    })
  })
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

type Started = { child: ChildProcess; url: string }

/** Starts the binary and waits until it answers; rejects at once when it exits early. */
async function start(binary: string, dir: string, prefix: string): Promise<Started> {
  const port = await freePort()
  const url = `http://127.0.0.1:${port}`
  const child = spawn(binary, ['-data-dir', dir, '-listen', `127.0.0.1:${port}`], {
    env: { ...process.env, GOTREE_ADMIN_USER: ADMIN.user, GOTREE_ADMIN_PASSWORD: ADMIN.password },
    stdio: ['ignore', 'ignore', 'pipe'],
  })
  let stderr = ''
  child.stderr!.on('data', (chunk: Buffer) => {
    const text = chunk.toString()
    stderr += text
    process.stderr.write(text.replace(/^(?=.)/gm, prefix))
  })
  let exited = false
  child.once('exit', () => {
    exited = true
  })
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (exited) throw new Error(`gotree exited before it was ready (port ${port}):\n${stderr}`)
    try {
      if ((await fetch(`${url}/api/health`)).ok) return { child, url }
    } catch {
      // not listening yet
    }
    await sleep(100)
  }
  child.kill('SIGKILL')
  throw new Error(`gotree was not ready on ${url} within 30 s:\n${stderr}`)
}

async function stop(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return
  const exit = new Promise<void>((resolve) => child.once('exit', () => resolve()))
  child.kill('SIGTERM')
  const timer = setTimeout(() => child.kill('SIGKILL'), 5_000)
  await exit
  clearTimeout(timer)
}

export const test = base.extend<object, { gotree: Server }>({
  // Each worker gets its own server (a free port) and data directory, with its own admin, so
  // the tests of different workers never share state. A worker restarted after a failure has a
  // new index and therefore a fresh database.
  gotree: [
    // The callback parameter is not called `use`, which the React hooks lint rule would mistake for a hook.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide, workerInfo) => {
      const dir = fileURLToPath(new URL(`../.e2e-data/w${workerInfo.workerIndex}`, import.meta.url))
      const binary = fileURLToPath(new URL('../../gotree', import.meta.url))
      if (!existsSync(binary)) throw new Error(`${binary} is missing; build it first: CGO_ENABLED=0 go build -o gotree ./cmd/gotree`)
      rmSync(dir, { recursive: true, force: true })
      let started: Started | undefined
      let lastError: unknown
      // A port taken between the probe and the listen makes the child exit early; try again.
      for (let attempt = 1; attempt <= 3 && !started; attempt++) {
        try {
          started = await start(binary, dir, `[gotree w${workerInfo.workerIndex}] `)
        } catch (e) {
          lastError = e
          rmSync(dir, { recursive: true, force: true })
        }
      }
      if (!started) throw lastError
      try {
        await provide({ url: started.url })
      } finally {
        await stop(started.child)
        rmSync(dir, { recursive: true, force: true })
      }
    },
    { scope: 'worker', timeout: 60_000 },
  ],

  baseURL: async ({ gotree }, provide) => provide(gotree.url),
  // The app defaults to Russian; the specs read the English source strings, so every context
  // starts with the hidden 'en' choice saved. The choice is kept per origin, and the origin has
  // the worker's port. Contexts made by hand (browser.newContext) get it too.
  // e2e/i18n.spec.ts switches to Russian and Kazakh on purpose.
  storageState: async ({ gotree }, provide) =>
    provide({ cookies: [], origins: [{ origin: gotree.url, localStorage: [{ name: 'gotree-lang', value: 'en' }] }] }),
})

export { expect }
