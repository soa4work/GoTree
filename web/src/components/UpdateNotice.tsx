import { useTranslation } from 'react-i18next'
import { useUpdateReady } from '../lib/updateReady'
import { Button } from './Button'

/** Offers a reload when a new version of GoTree has been installed. */
export function UpdateNotice() {
  const { t } = useTranslation()
  const ready = useUpdateReady()

  return (
    <div role="status" className={ready ? 'fixed inset-x-4 bottom-24 z-40 md:bottom-6 md:left-auto md:max-w-sm print:hidden' : 'sr-only'}>
      {ready && (
        <div className="flex items-center justify-between gap-3 rounded-xl bg-slate-900 px-4 py-3 text-white shadow-lg dark:bg-slate-100 dark:text-slate-900">
          <span>{t('app.updateReady')}</span>
          <Button variant="secondary" onClick={() => window.dispatchEvent(new CustomEvent('gotree:apply-update'))}>
            {t('app.reload')}
          </Button>
        </div>
      )}
    </div>
  )
}
