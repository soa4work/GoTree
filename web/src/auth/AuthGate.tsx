import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { getAuthState } from '../api/client'
import { UpdateNotice } from '../components/UpdateNotice'
import { LoginPage } from './LoginPage'
import { SetupPage } from './SetupPage'

/** Shows setup or login until there is a session, then the app. */
export function AuthGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })

  if (auth.isPending) {
    return (
      <p role="status" className="p-6 text-slate-700 dark:text-slate-300">
        {t('app.loading')}
      </p>
    )
  }
  if (auth.isError) {
    return (
      <main id="main" className="p-6">
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {t('dashboard.offline')}
        </p>
      </main>
    )
  }
  if (auth.data.setupRequired) return (
      <>
        <SetupPage />
        <UpdateNotice />
      </>
    )
  if (!auth.data.user) return (
      <>
        <LoginPage />
        <UpdateNotice />
      </>
    )
  return <>{children}</>
}
