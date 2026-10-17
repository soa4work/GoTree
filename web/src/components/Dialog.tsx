import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
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
  // Bumped to put the confirmation back on top after the browser closed this dialog.
  const [confirmKey, setConfirmKey] = useState(0)
  // Callers pass a new onClose every render; the guard must stay the same object.
  const onCloseRef = useRef(onClose)
  useEffect(() => {
    onCloseRef.current = onClose
  })
  // Closed from outside (e.g. after saving): forget a pending question.
  if (!open && confirming) setConfirming(false)

  const register = useCallback((isDirty: () => boolean) => {
    check.current = isDirty
    return () => {
      if (check.current === isDirty) check.current = null
    }
  }, [])
  const requestClose = useCallback(() => {
    if (check.current?.()) setConfirming(true)
    else onCloseRef.current()
  }, [])
  const guard = useMemo(() => ({ register, requestClose }), [register, requestClose])

  return (
    <>
      <DialogShell open={open} onClose={requestClose} onReopen={() => setConfirmKey((k) => k + 1)} title={title} wide={wide}>
        <GuardContext.Provider value={guard}>{children}</GuardContext.Provider>
      </DialogShell>
      <ConfirmDialog
        key={confirmKey}
        open={open && confirming}
        title={t('common.discardTitle')}
        message={t('common.discardMessage')}
        confirmLabel={t('common.discard')}
        onConfirm={() => {
          setConfirming(false)
          onCloseRef.current()
        }}
        onClose={() => setConfirming(false)}
      />
    </>
  )
}
