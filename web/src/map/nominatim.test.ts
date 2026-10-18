import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { LOOKUP_GAP_MS, lookupPlace, resetLookupThrottle } from './nominatim'

const fetchMock = vi.fn(() => Promise.resolve(new Response('[]', { status: 200 })))

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockClear()
  resetLookupThrottle()
})
afterEach(async () => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('en')
})

describe('lookupPlace', () => {
  it('queues lookups for different places one gap apart', async () => {
    const first = lookupPlace('Leipzig')
    const second = lookupPlace('Dresden')
    await first
    expect(fetchMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(LOOKUP_GAP_MS - 1)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    await second
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('asks for names in the interface language', async () => {
    await i18n.changeLanguage('ru')
    await lookupPlace('Leipzig')
    const calls = fetchMock.mock.calls as unknown as [string][]
    expect(calls[0]![0]).toContain('accept-language=ru')
  })

  it('does not fetch when a queued call is aborted', async () => {
    await lookupPlace('Leipzig')
    const ctl = new AbortController()
    const queued = lookupPlace('Dresden', ctl.signal)
    const rejected = expect(queued).rejects.toBeDefined()
    ctl.abort()
    await rejected
    await vi.advanceTimersByTimeAsync(LOOKUP_GAP_MS * 2)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('gives an aborted last slot back to the next lookup', async () => {
    await lookupPlace('Leipzig')
    const ctl = new AbortController()
    const queued = lookupPlace('Dresden', ctl.signal)
    const rejected = expect(queued).rejects.toBeDefined()
    ctl.abort()
    await rejected
    const next = lookupPlace('Berlin')
    await vi.advanceTimersByTimeAsync(LOOKUP_GAP_MS)
    await next
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
