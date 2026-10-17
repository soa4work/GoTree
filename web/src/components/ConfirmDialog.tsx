import { useTranslation } from 'react-i18next'
import { Button } from './Button'
import { DialogShell } from './DialogShell'

interface Props {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  busy?: boolean
  error?: string | null
  onConfirm: () => void
  onClose: () => void
}

export function ConfirmDialog({ open, title, message, confirmLabel, busy, error, onConfirm, onClose }: Props) {
  const { t } = useTranslation()
  return (
    <DialogShell open={open} onClose={onClose} title={title}>
      <p className="text-slate-700 dark:text-slate-300">{message}</p>
      {error && (
        <p role="alert" className="mt-3 font-medium text-red-700 dark:text-red-400">
          {error}
        </p>
      )}
      <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button variant="secondary" onClick={onClose}>
          {t('common.cancel')}
        </Button>
        <Button variant="danger" onClick={onConfirm} busy={busy}>
          {confirmLabel}
        </Button>
      </div>
    </DialogShell>
  )
}
