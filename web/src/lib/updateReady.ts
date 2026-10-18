import { useSyncExternalStore } from 'react'

// The service worker can report a new version before any component has
// mounted (e.g. while the auth state loads), so the flag lives outside React.
let ready = false
const listeners = new Set<() => void>()

export function markUpdateReady() {
  ready = true
  listeners.forEach((l) => l())
}

export function useUpdateReady(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => {
        listeners.delete(l)
      }
    },
    () => ready,
    () => false,
  )
}

/** Tests only. */
export function resetUpdateReady() {
  ready = false
  listeners.forEach((l) => l())
}
