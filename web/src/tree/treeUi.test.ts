import { describe, expect, it, vi } from 'vitest'
import { createTreeUiStore } from './treeUi'

describe('tree ui store', () => {
  it('does not notify when nothing changed', () => {
    const store = createTreeUiStore({ selectedId: 1, focusId: 2 })
    const listener = vi.fn()
    store.subscribe(listener)
    store.set({ selectedId: 1 })
    store.set({ selectedId: 1, focusId: 2 })
    store.set({})
    expect(listener).not.toHaveBeenCalled()
  })

  it('notifies once per change and stops after unsubscribe', () => {
    const store = createTreeUiStore({ selectedId: null, focusId: 2 })
    const listener = vi.fn()
    const off = store.subscribe(listener)
    store.set({ selectedId: 3, focusId: 4 })
    expect(listener).toHaveBeenCalledTimes(1)
    expect(store.get()).toEqual({ selectedId: 3, focusId: 4 })
    off()
    store.set({ selectedId: 5 })
    expect(listener).toHaveBeenCalledTimes(1)
  })
})
