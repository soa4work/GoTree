import { Trans } from 'react-i18next'

const link = 'underline hover:text-slate-900 dark:hover:text-white'

/** The CC BY 4.0 attribution the logo needs wherever it is shown. */
export function GopherCredit({ className = '' }: { className?: string }) {
  return (
    <p className={`text-xs text-slate-600 dark:text-slate-400 ${className}`}>
      <Trans
        i18nKey="app.gopherCredit"
        components={{
          // No content here: Trans fills it in.
          // eslint-disable-next-line jsx-a11y/anchor-has-content
          author: <a href="https://reneefrench.blogspot.com/" className={link} />,
          // No content here: Trans fills it in.
          // eslint-disable-next-line jsx-a11y/anchor-has-content
          license: <a href="https://creativecommons.org/licenses/by/4.0/" className={link} />,
        }}
      />
    </p>
  )
}
