import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, getJSON, NetworkError, uploadFile } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('a request that gets no answer', () => {
  it('rejects with a NetworkError, whatever wording the browser uses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Load failed')))
    await expect(getJSON('/persons')).rejects.toBeInstanceOf(NetworkError)
    await expect(uploadFile('/import/gedcom', new File(['x'], 'a.ged'))).rejects.toBeInstanceOf(NetworkError)
  })

  it('lets an abort through, because React Query relies on it', async () => {
    const abort = new DOMException('The operation was aborted.', 'AbortError')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(abort))
    await expect(getJSON('/persons')).rejects.toBe(abort)
  })

  it('still reports an answer with an error status as an ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 502, statusText: 'Bad Gateway' })))
    await expect(getJSON('/persons')).rejects.toBeInstanceOf(ApiError)
  })
})
