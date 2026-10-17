import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from './ConfirmDialog'
import { GuardContext } from './dialogGuard'
import { DialogShell } from './DialogShell'

interface Props {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  /** Wide dialogs for forms with many fields. */
  wide?: boolean
}

/**
 * A modal for content and forms. Closing it by Escape, the × button or a
 * form's Cancel asks "Discard changes?" while a Form inside has unsaved input;
 * closing after a successful save calls onClose directly and never asks.
 */
export function Dialog({ open, onClose, title, children, wide }: Props) {
  const { t } = useTranslation()
  const check = useRef<(() => boolean) | null>(null)
  const [confirming, setConfirming] = useState(false)

  const register = useCallback((isDirty: () => boolean) => {
    check.current = isDirty
    return () => {
      if (check.current === isDirty) check.current = null
    }
  }, [])
  const requestClose = useCallback(() => {
    if (check.current?.()) setConfirming(true)
    else onClose()
  }, [onClose])
  const guard = useMemo(() => ({ register, requestClose }), [register, requestClose])

  return (
    <>
      <DialogShell open={open} onClose={requestClose} title={title} wide={wide}>
        <GuardContext.Provider value={guard}>{children}</GuardContext.Provider>
      </DialogShell>
      <ConfirmDialog
        open={open && confirming}
        title={t('common.discardTitle')}
        message={t('common.discardMessage')}
        confirmLabel={t('common.discard')}
        onConfirm={() => {
          setConfirming(false)
          onClose()
        }}
        onClose={() => setConfirming(false)}
      />
    </>
  )
}
