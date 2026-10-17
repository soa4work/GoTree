import { useCallback, useSyncExternalStore } from 'react'

/** Tailwind's md breakpoint (48rem), so JS and the md: classes switch together at any font size. */
export const mdQuery = '(min-width: 48rem)'

/** Tailwind's lg breakpoint (64rem), for layouts that follow the lg: classes. */
export const lgQuery = '(min-width: 64rem)'

/** Whether a CSS media query matches; false where matchMedia is missing (jsdom). */
export function useMediaQuery(query: string) {
  const subscribe = useCallback(
    (notify: () => void) => {
      const mq = window.matchMedia?.(query)
      mq?.addEventListener('change', notify)
      return () => mq?.removeEventListener('change', notify)
    },
    [query],
  )
  return useSyncExternalStore(subscribe, () => window.matchMedia?.(query).matches ?? false)
}
