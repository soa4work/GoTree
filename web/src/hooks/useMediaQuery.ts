import { useSyncExternalStore } from 'react'

/** Whether a CSS media query matches; false where matchMedia is missing (jsdom). */
export function useMediaQuery(query: string) {
  return useSyncExternalStore(
    (notify) => {
      const mq = window.matchMedia?.(query)
      mq?.addEventListener('change', notify)
      return () => mq?.removeEventListener('change', notify)
    },
    () => window.matchMedia?.(query).matches ?? false,
  )
}
