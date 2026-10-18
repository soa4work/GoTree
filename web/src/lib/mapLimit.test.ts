import { describe, expect, it } from 'vitest'
import { mapLimit } from './mapLimit'

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

const tick = () => new Promise((r) => setTimeout(r, 0))

describe('mapLimit', () => {
  it('keeps at most `limit` calls in flight and the input order', async () => {
    const gates = Array.from({ length: 10 }, deferred)
    let inFlight = 0
    let peak = 0
    const run = mapLimit([...gates.keys()], 3, async (i) => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await gates[i]!.promise
      inFlight--
      return i * 2
    })
    await tick()
    expect(inFlight).toBe(3)
    // Finish them in reverse to show the results are not in completion order.
    for (let i = 9; i >= 0; i--) {
      gates[i]!.resolve()
      await tick()
    }
    expect(await run).toEqual([0, 2, 4, 6, 8, 10, 12, 14, 16, 18])
    expect(peak).toBe(3)
  })

  it('returns an empty list for no input', async () => {
    expect(await mapLimit([], 4, async () => 1)).toEqual([])
  })

  it('treats a limit below one as one', async () => {
    let inFlight = 0
    let peak = 0
    const out = await mapLimit([1, 2, 3], 0, async (n) => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await tick()
      inFlight--
      return n
    })
    expect(out).toEqual([1, 2, 3])
    expect(peak).toBe(1)
  })
})
