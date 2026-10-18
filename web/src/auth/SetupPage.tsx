import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { setup } from '../api/client'
import { Button } from '../components/Button'
import { TextField } from '../components/TextField'
import { AuthCard } from './AuthCard'
import { FormError } from './FormError'
import { errorText, fieldErrors } from '../lib/errors'

export function SetupPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [treeName, setTreeName] = useState('')
  const [mismatch, setMismatch] = useState(false)

  const mutation = useMutation({
    mutationFn: setup,
    onSuccess: (state) => queryClient.setQueryData(['auth'], state),
  })
  const fields = fieldErrors(mutation.error, t)
  const formError =
    mutation.error && Object.keys(fields).length === 0 ? t('auth.setupFailed', { message: errorText(mutation.error, t) }) : null

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (password !== confirm) {
      setMismatch(true)
      return
    }
    setMismatch(false)
    mutation.mutate({ username, password, treeName })
  }

  return (
    <AuthCard title={t('auth.setupTitle')} intro={t('auth.setupIntro')}>
      <form onSubmit={submit} noValidate className="space-y-4">
        <FormError message={formError} />
        <TextField
          label={t('auth.treeName')}
          hint={t('auth.treeNameHint')}
          value={treeName}
          onChange={(e) => setTreeName(e.target.value)}
          placeholder={t('auth.treeNamePlaceholder')}
          error={fields.treeName}
        />
        <TextField
          label={t('auth.username')}
          autoComplete="username"
          required
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          error={fields.username}
        />
        <TextField
          label={t('auth.password')}
          hint={t('auth.passwordHint')}
          type="password"
          autoComplete="new-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={fields.password}
        />
        <TextField
          label={t('auth.confirmPassword')}
          type="password"
          autoComplete="new-password"
          required
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          error={mismatch ? t('auth.passwordMismatch') : undefined}
        />
        <Button type="submit" busy={mutation.isPending} className="w-full">
          {t('auth.createAccount')}
        </Button>
      </form>
    </AuthCard>
  )
}
