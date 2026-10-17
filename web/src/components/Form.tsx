import { useEffect, useRef, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FormError } from '../auth/FormError'
import { Button } from './Button'
import { useDialogGuard } from './dialogGuard'

interface Props {
  onSubmit: () => void
  onCancel: () => void
  submitLabel: string
  busy?: boolean
  error?: string | null
  children: ReactNode
  /** Extra actions on the left, e.g. Delete. */
  secondary?: ReactNode
}

/** A dialog form: Ctrl/Cmd+Enter submits from any field. */
export function Form({ onSubmit, onCancel, submitLabel, busy, error, children, secondary }: Props) {
  const { t } = useTranslation()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSubmit()
  }
  const ref = useRef<HTMLFormElement>(null)
  // Inside a Dialog, Cancel goes through its unsaved-changes guard; onCancel is the fallback outside one.
  const guard = useDialogGuard()
  const initial = useRef<string | null>(null)
  // Dirty = the fields differ from how they were when the form mounted. Read
  // from the DOM so every dialog gets it without wiring its own state in; state
  // that is not rendered as a form field is not tracked.
  useEffect(() => {
    if (ref.current) initial.current = fieldsSignature(ref.current)
  }, [])
  const register = guard?.register
  useEffect(() => {
    const form = ref.current
    if (!form) return
    return register?.(() => initial.current !== null && fieldsSignature(form) !== initial.current)
  }, [register])
  useEffect(() => {
    const form = ref.current
    if (!form) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        form.requestSubmit()
      }
    }
    form.addEventListener('keydown', onKeyDown)
    return () => form.removeEventListener('keydown', onKeyDown)
  }, [])
  return (
    <form ref={ref} onSubmit={submit} noValidate className="space-y-4">
      <FormError message={error ?? null} />
      {children}
      <div className="flex flex-col-reverse gap-2 pt-2 sm:flex-row sm:items-center sm:justify-between">
        <div>{secondary}</div>
        <div className="flex flex-col-reverse gap-2 sm:flex-row">
          <Button type="button" variant="secondary" onClick={guard ? guard.requestClose : onCancel}>            {t('common.cancel')}
          </Button>
          <Button type="submit" busy={busy}>
            {submitLabel}
            <span className="sr-only"> ({t('shortcuts.ctrlEnter')})</span>
          </Button>
        </div>
      </div>
    </form>
  )
}

/** The values of all fields, in order; adding or removing a field changes it too. */
function fieldsSignature(form: HTMLFormElement): string {
  return JSON.stringify(
    Array.from(form.elements, (el) => {
      if (el instanceof HTMLInputElement) {
        if (el.type === 'checkbox' || el.type === 'radio') return el.checked
        if (el.type === 'file') return el.files?.length ?? 0
        return el.value
      }
      if (el instanceof HTMLSelectElement || el instanceof HTMLTextAreaElement) return el.value
      return null
    }),
  )
}
