import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, login } from '../api/client'
import { Button } from '../components/Button'
import { TextField } from '../components/TextField'
import { AuthCard } from './AuthCard'
import { FormError } from './FormError'
import { errorText } from '../lib/errors'

export function LoginPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  const mutation = useMutation({
    mutationFn: () => login(username, password),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['auth'] }),
  })

  let formError: string | null = null
  if (mutation.error instanceof ApiError) {
    if (mutation.error.status === 401) formError = t('auth.badCredentials')
    else if (mutation.error.status === 429) formError = t('auth.throttled')
    else formError = t('auth.loginFailed', { message: errorText(mutation.error, t) })
  } else if (mutation.error) {
    formError = t('auth.loginFailed', { message: errorText(mutation.error, t) })
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    mutation.mutate()
  }

  return (
    <AuthCard title={t('auth.loginTitle')}>
      <form onSubmit={submit} className="space-y-4">
        <FormError message={formError} />
        <TextField
          label={t('auth.username')}
          autoComplete="username"
          required
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <TextField
          label={t('auth.password')}
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <Button type="submit" busy={mutation.isPending} className="w-full">
          {t('auth.logIn')}
        </Button>
      </form>
    </AuthCard>
  )
}
