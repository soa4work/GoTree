import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { AuthCard } from '../auth/AuthCard'
import { useLogout } from '../auth/useLogout'
import { Button } from '../components/Button'
import { TreePicker } from './TreePicker'

/** What a signed-in user sees while no tree is open, for example after being removed from theirs. */
export function NoTreePage() {
  const { t } = useTranslation()
  const logout = useLogout()
  return (
    <AuthCard title={t('trees.noTreeTitle')} intro={t('trees.noTreeText')}>
      <TreePicker />
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
