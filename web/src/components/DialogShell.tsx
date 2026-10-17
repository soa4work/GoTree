import { useEffect, useId, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

interface Props {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  /** Wide dialogs for forms with many fields. */
  wide?: boolean
}

/**
 * The bare modal, without the unsaved-changes guard. Dialog wraps it; ConfirmDialog
 * uses it directly so the two do not import each other.
 *
 * A modal built on the native <dialog>: the browser provides the focus trap,
 * Escape to close, an inert background and focus return to the trigger.
 */
export function DialogShell({ open, onClose, title, children, wide }: Props) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDialogElement>(null)
  const titleId = useId()

  useEffect(() => {
    const el = ref.current
    if (!el) return
    if (open && !el.open) {
      el.showModal()
      // Forms start in their first field (WAI-ARIA dialog pattern); dialogs
      // without fields keep the browser's choice, the first button.
      el.querySelector<HTMLElement>('[data-dialog-body] :is(input, select, textarea):not([disabled])')?.focus()
    }
    if (!open && el.open) el.close()
  }, [open])

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
      className={[
        'm-0 mt-auto max-h-[92dvh] w-full max-w-none rounded-t-2xl bg-white p-0 text-slate-900 shadow-xl backdrop:bg-slate-950/50',
        'sm:m-auto sm:rounded-2xl dark:bg-slate-900 dark:text-slate-100',
        wide ? 'sm:max-w-2xl' : 'sm:max-w-lg',
      ].join(' ')}
    >
      {open && (
        <div className="flex max-h-[92dvh] flex-col">
          <div className="flex items-center justify-between gap-4 border-b border-slate-200 px-5 py-3 dark:border-slate-800">
            <h2 id={titleId} className="text-lg font-semibold">
              {title}
            </h2>
            <button
              type="button"
              onClick={onClose}
              className="inline-flex size-11 items-center justify-center rounded-lg text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
            >
              <span aria-hidden="true" className="text-2xl leading-none">
                ×
              </span>
              <span className="sr-only">{t('common.close')}</span>
            </button>
          </div>
          <div data-dialog-body className="overflow-y-auto px-5 py-4">
            {children}
          </div>
        </div>
      )}
    </dialog>
  )
}
