import { useCallback, useState } from 'react'

export type CopyState = 'idle' | 'copied' | 'failed'

/**
 * Copies text to the clipboard and reports the outcome. `navigator.clipboard`
 * is missing over plain http (not a secure context), which is common for a
 * self-hosted install on a home network, and the browser may also refuse.
 */
export function useCopy() {
  const [state, setState] = useState<CopyState>('idle')
  const copy = useCallback(async (text: string) => {
    try {
      if (!navigator.clipboard) throw new Error('no clipboard')
      await navigator.clipboard.writeText(text)
      setState('copied')
    } catch {
      setState('failed')
    }
  }, [])
  const reset = useCallback(() => setState('idle'), [])
  return { state, copy, reset }
}
