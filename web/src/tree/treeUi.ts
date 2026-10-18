import { createContext, useContext, useSyncExternalStore, type KeyboardEvent } from 'react'

export interface TreeUiState {
  selectedId: number | null
  focusId: number | null
}

export interface TreeActions {
  select(id: number): void
  center(id: number): void
  key(id: number, e: KeyboardEvent<HTMLButtonElement>): void
  jump(id: number): void
}

/**
 * Selection and the single tab stop live outside the node data so that a
 * change redraws only the cards whose own value flipped.
 */
export function createTreeUiStore(init: TreeUiState) {
  let s = init
  const ls = new Set<() => void>()
  return {
    get: () => s,
    set(p: Partial<TreeUiState>) {
      const n = { ...s, ...p }
      if (n.selectedId === s.selectedId && n.focusId === s.focusId) return
      s = n
      ls.forEach((l) => l())
    },
    subscribe(l: () => void) {
      ls.add(l)
      return () => {
        ls.delete(l)
      }
    },
  }
}

export type TreeUiStore = ReturnType<typeof createTreeUiStore>

export const TreeUiContext = createContext<{
  store: TreeUiStore
  actions: TreeActions
} | null>(null)

function useCtx() {
  const c = useContext(TreeUiContext)
  if (!c) throw new Error('TreeUiContext is missing')
  return c
}

/** Selectors must return primitives, so a card re-renders only when its value flips. */
export function useTreeUi<T>(select: (s: TreeUiState) => T): T {
  const { store } = useCtx()
  return useSyncExternalStore(
    store.subscribe,
    () => select(store.get()),
    () => select(store.get()),
  )
}

export function useTreeActions() {
  return useCtx().actions
}
