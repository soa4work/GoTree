import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from './Button'
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
 *
 * The question replaces the dialog's content instead of opening a second
 * <dialog>, so the browser only ever has one close watcher to deal with. The
 * form stays mounted (hidden) and keeps its input.
 */
export function Dialog({ open, onClose, title, children, wide }: Props) {
  const { t } = useTranslation()
  const check = useRef<(() => boolean) | null>(null)
  const [confirming, setConfirming] = useState(false)
  const confirmingRef = useRef(false)
  const questionRef = useRef<HTMLDivElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
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
  const ask = (value: boolean) => {
    confirmingRef.current = value
    setConfirming(value)
  }
  const requestClose = useCallback(() => {
    if (confirmingRef.current) {
      // Escape or × on the question means "keep editing".
      confirmingRef.current = false
      setConfirming(false)
    } else if (check.current?.()) {
      confirmingRef.current = true
      setConfirming(true)
    } else {
      onCloseRef.current()
    }
  }, [])
  const guard = useMemo(() => ({ register, requestClose }), [register, requestClose])

  // Focus moves to the question, and back to the form when it is dismissed.
  const wasConfirming = useRef(false)
  useEffect(() => {
    if (confirming) questionRef.current?.querySelector('button')?.focus()
    else if (wasConfirming.current) {
      bodyRef.current?.querySelector<HTMLElement>(':is(input, select, textarea):not([disabled])')?.focus()
    }
    wasConfirming.current = confirming
  }, [confirming])

  return (
    <DialogShell open={open} onClose={requestClose} title={confirming ? t('common.discardTitle') : title} wide={wide}>
      <GuardContext.Provider value={guard}>
        <div ref={bodyRef} hidden={confirming}>
          {children}
        </div>
      </GuardContext.Provider>
      {confirming && (
        <div ref={questionRef}>
          <p className="text-slate-700 dark:text-slate-300">{t('common.discardMessage')}</p>
          <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button variant="secondary" onClick={requestClose}>
              {t('common.keepEditing')}
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                ask(false)
                onCloseRef.current()
              }}
            >
              {t('common.discard')}
            </Button>
          </div>
        </div>
      )}
    </DialogShell>
  )
}
