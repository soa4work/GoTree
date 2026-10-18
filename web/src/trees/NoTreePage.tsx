import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError, getAuthState } from '../api/client'
import { AuthCard } from '../auth/AuthCard'
import { useLogout } from '../auth/useLogout'
import { Button } from '../components/Button'
import { TreePicker } from './TreePicker'

/** What a signed-in user sees while no tree is open, for example after being removed from theirs. */
export function NoTreePage() {
  const { t } = useTranslation()
  const logout = useLogout()
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  return (
    <AuthCard title={t('trees.noTreeTitle')} intro={t('trees.noTreeText')}>
      <TreePicker />
      {auth.data?.user?.role === 'admin' && (
        <Link to="/admin/users" className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100">
          {t('accounts.manage')}
        </Link>
      )}
      {logout.isError && !(logout.error instanceof ApiError && logout.error.status === 401) && (
        <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
          {t('app.logOutFailed')}
        </p>
      )}
      <Button variant="ghost" onClick={() => logout.mutate()} busy={logout.isPending}>
        {t('app.logOut')}
      </Button>
    </AuthCard>
  )
}
