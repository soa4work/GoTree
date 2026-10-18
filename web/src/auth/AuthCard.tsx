import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { GopherCredit } from '../components/GopherCredit'
import { LanguageSwitch } from '../components/LanguageSwitch'
import { PageHeading } from '../components/PageHeading'

export function AuthCard({ title, intro, children }: { title: string; intro?: string; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <main
      id="main"
      className="flex min-h-dvh items-start justify-center bg-slate-50 px-4 py-10 text-slate-900 sm:items-center dark:bg-slate-950 dark:text-slate-100"
    >
      <div className="w-full max-w-md space-y-6 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm sm:p-8 dark:border-slate-800 dark:bg-slate-900">
        <div className="flex items-center gap-3">
          <img src="/icons/icon-192.png" alt="" width={48} height={48} className="size-12 rounded-lg" />
          <span className="text-xl font-semibold text-brand-700 dark:text-brand-100">{t('app.name')}</span>
          <div className="ml-auto">
            <LanguageSwitch />
          </div>
        </div>
        <div className="space-y-2">
          <PageHeading title={title}>{title}</PageHeading>
          {intro && <p className="text-slate-700 dark:text-slate-300">{intro}</p>}
        </div>
        {children}
        <GopherCredit className="border-t border-slate-200 pt-4 dark:border-slate-800" />
      </div>
    </main>
  )
}
