import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { changePassword, endSession, listSessions, logoutEverywhere, type SessionInfo } from '../api/client'
import { FormError } from '../auth/FormError'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { formatDay } from '../lib/dates'
import { errorText, fieldErrors } from '../lib/errors'

export function AccountPage() {
  const { t } = useTranslation()
  return (
    <div className="max-w-3xl space-y-8">
      <PageHeading title={t('nav.account')}>{t('nav.account')}</PageHeading>
      <PasswordSection />
      <SessionsSection />
    </div>
  )
}

function PasswordSection() {
  const { t } = useTranslation()
  const headingId = useId()
  const queryClient = useQueryClient()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [mismatch, setMismatch] = useState(false)
  const [changed, setChanged] = useState(false)

  const mutation = useMutation({
    mutationFn: () => changePassword(current, next),
    onSuccess: () => {
      setCurrent('')
      setNext('')
      setConfirm('')
      setChanged(true)
      void queryClient.invalidateQueries({ queryKey: ['sessions'] })
    },
  })
  const fields = fieldErrors(mutation.error, t)
  const formError =
    mutation.error && Object.keys(fields).length === 0
      ? t('account.passwordFailed', { message: errorText(mutation.error, t) })
      : null

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setChanged(false)
    if (next !== confirm) {
      setMismatch(true)
      return
    }
    setMismatch(false)
    mutation.mutate()
  }

  return (
    <section aria-labelledby={headingId} className="space-y-4">
      <h2 id={headingId} className="text-lg font-semibold">
        {t('account.passwordTitle')}
      </h2>
      <form onSubmit={submit} noValidate className="max-w-md space-y-4">
        <FormError message={formError} />
        <TextField
          label={t('account.currentPassword')}
          type="password"
          autoComplete="current-password"
          required
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          error={fields.currentPassword}
        />
        <TextField
          label={t('account.newPassword')}
          hint={t('auth.passwordHint')}
          type="password"
          autoComplete="new-password"
          required
          value={next}
          onChange={(e) => setNext(e.target.value)}
          error={fields.newPassword}
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
        <Button type="submit" busy={mutation.isPending}>
          {t('account.passwordTitle')}
        </Button>
        {changed && <p role="status">{t('account.passwordChanged')}</p>}
      </form>
    </section>
  )
}

function SessionsSection() {
  const { t } = useTranslation()
  const headingId = useId()
  const queryClient = useQueryClient()
  const [confirming, setConfirming] = useState(false)
  const sessions = useQuery({ queryKey: ['sessions'], queryFn: ({ signal }) => listSessions(signal) })
  const end = useMutation({
    mutationFn: (id: number) => endSession(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['sessions'] }),
  })
  const everywhere = useMutation({
    mutationFn: logoutEverywhere,
    onSuccess: () => {
      setConfirming(false)
      // Same as the header's log out: drop what this user loaded, keep the
      // auth query mounted so its refetch switches to the login screen.
      queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== 'auth' })
      void queryClient.invalidateQueries({ queryKey: ['auth'] })
    },
  })

  return (
    <section aria-labelledby={headingId} className="space-y-4">
      <h2 id={headingId} className="text-lg font-semibold">
        {t('account.sessionsTitle')}
      </h2>
      <p className="text-slate-700 dark:text-slate-300">{t('account.sessionsIntro')}</p>
      {(sessions.isPending || sessions.isError) && (
        <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
          {sessions.isPending ? t('app.loading') : t('common.loadFailed')}
        </p>
      )}
      {end.error && <FormError message={t('common.saveFailed', { message: errorText(end.error, t) })} />}
      {sessions.data && (
        <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
          {sessions.data.map((s) => (
            <SessionRow key={s.id} session={s} busy={end.isPending && end.variables === s.id} onEnd={() => end.mutate(s.id)} />
          ))}
        </ul>
      )}
      <Button variant="secondary" onClick={() => setConfirming(true)}>
        {t('account.signOutEverywhere')}
      </Button>
      <ConfirmDialog
        open={confirming}
        title={t('account.signOutEverywhereTitle')}
        message={t('account.signOutEverywhereMessage')}
        confirmLabel={t('account.signOutEverywhere')}
        busy={everywhere.isPending}
        error={everywhere.error ? t('common.saveFailed', { message: errorText(everywhere.error, t) }) : null}
        onConfirm={() => everywhere.mutate()}
        onClose={() => setConfirming(false)}
      />
    </section>
  )
}

function SessionRow({ session: s, busy, onEnd }: { session: SessionInfo; busy: boolean; onEnd: () => void }) {
  const { t } = useTranslation()
  const device = s.userAgent || t('account.unknownDevice')
  return (
    <li className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
      <div className="min-w-0">
        <p className="break-words text-sm">
          {device}
          {s.current && (
            <span className="ml-2 rounded bg-slate-200 px-1.5 py-0.5 text-xs font-semibold dark:bg-slate-800">
              {t('account.thisDevice')}
            </span>
          )}
        </p>
        <p className="text-sm text-slate-600 dark:text-slate-400">
          {t('account.signedIn', { date: formatDay(s.createdAt) })} · {t('account.lastActive', { date: formatDay(s.lastSeenAt) })}
        </p>
      </div>
      {!s.current && (
        <Button variant="ghost" onClick={onEnd} busy={busy}>
          {t('account.signOut')}
          <span className="sr-only">: {device}</span>
        </Button>
      )}
    </li>
  )
}
