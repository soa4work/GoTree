import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

const action = 'inline-flex min-h-11 items-center justify-center rounded-lg px-4 text-center font-medium'

/** Shown wherever the tree has no people yet: add yourself or bring a tree in. */
export function OnboardingCard() {
  const { t } = useTranslation()
  const id = useId()
  return (
    <section aria-labelledby={id} className="rounded-xl border border-dashed border-slate-300 p-6 dark:border-slate-700">
      <h2 id={id} className="text-xl font-semibold">
        {t('onboarding.title')}
      </h2>
      <p className="mt-2 text-slate-700 dark:text-slate-300">{t('onboarding.text')}</p>
      <div className="mt-4 flex flex-wrap gap-3">
        <Link to="/people?new=1" className={`${action} bg-brand-700 text-white hover:bg-brand-800`}>
          {t('onboarding.addYourself')}
        </Link>
        <Link
          to="/import-export"
          className={`${action} border border-slate-400 text-slate-800 hover:bg-slate-100 dark:border-slate-600 dark:text-slate-100 dark:hover:bg-slate-800`}
        >
          {t('onboarding.import')}
        </Link>
      </div>
    </section>
  )
}
