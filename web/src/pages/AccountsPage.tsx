import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError, getAuthState } from '../api/client'
import {
  createAccount,
  deleteAccount,
  listAccounts,
  resetPassword,
  updateAccount,
  type Account,
  type AccountRole,
} from '../api/endpoints'
import { Button } from '../components/Button'
import { Choice } from '../components/Choice'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { Form } from '../components/Form'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { formatDay } from '../lib/dates'
import { errorText, formErrors } from '../lib/errors'

const roles: AccountRole[] = ['admin', 'user']
const label = (a: { displayName: string; username: string }) => a.displayName || a.username

export function AccountsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: ({ signal }) => listAccounts(signal) })
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Account | null>(null)
  const [resetting, setResetting] = useState<Account | null>(null)
  const [deleting, setDeleting] = useState<Account | null>(null)
  const [created, setCreated] = useState<Account | null>(null)
  const [passwordReset, setPasswordReset] = useState<Account | null>(null)
  const listRef = useRef<HTMLElement>(null)
  const selfId = auth.data?.user?.id
  const adminOnly = accounts.error instanceof ApiError && accounts.error.status === 403

  const remove = useMutation({
    mutationFn: (id: number) => deleteAccount(id),
    onSuccess: () => {
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey: ['accounts'] })
      // The deleted row took the button that opened the dialog with it, so the
      // dialog cannot return the focus there. Wait for it to close first.
      setTimeout(() => listRef.current?.focus())
    },
  })

  const list = accounts.data ?? []
  return (
    <div className="max-w-3xl space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeading title={t('nav.accounts')}>{t('nav.accounts')}</PageHeading>
        {!adminOnly && (
          <Button
            onClick={() => {
              setCreated(null)
              setPasswordReset(null)
              setCreating(true)
            }}
          >
            + {t('accounts.new')}
          </Button>
        )}
      </div>
      <p className="text-slate-700 dark:text-slate-300">{t('accounts.intro')}</p>

      {created && (
        <p role="status" className="font-medium">
          {t('accounts.created', { name: label(created) })} {t('accounts.createdNext')}{' '}
          <Link to="/members" className="text-brand-700 underline dark:text-brand-100">
            {t('accounts.openMembers')}
          </Link>
        </p>
      )}
      {passwordReset && (
        <p role="status" className="font-medium">
          {t('accounts.passwordReset', { name: label(passwordReset) })}
        </p>
      )}

      <section ref={listRef} tabIndex={-1} aria-label={t('nav.accounts')} className="space-y-3 focus:outline-none">
        <p role={accounts.isError && !adminOnly ? 'alert' : 'status'} className="text-sm text-slate-600 dark:text-slate-400">
          {accounts.isPending
            ? t('app.loading')
            : adminOnly
              ? t('accounts.adminOnly')
              : accounts.isError
                ? t('common.loadFailed')
                : t('accounts.count', { count: list.length })}
        </p>
        {list.length > 0 && (
          <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
            {list.map((a) => (
              <li key={a.id} className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
                <div className="min-w-0 space-y-0.5">
                  <p className="break-words font-medium">
                    {label(a)}
                    {a.role === 'admin' && (
                      <span className="ml-2 rounded bg-slate-200 px-1.5 py-0.5 text-xs font-semibold dark:bg-slate-800">
                        {t('accounts.admin')}
                      </span>
                    )}
                  </p>
                  <p className="text-sm text-slate-600 dark:text-slate-400">@{a.username}</p>
                  <p className="text-sm text-slate-600 dark:text-slate-400">
                    {a.trees.length > 0
                      ? a.trees.map((tr) => `${tr.name} (${t(`roles.${tr.role}`)})`).join(', ')
                      : t('accounts.noTrees')}
                  </p>
                  <p className="text-sm text-slate-600 dark:text-slate-400">
                    {a.lastSeenAt ? t('accounts.lastSeen', { date: formatDay(a.lastSeenAt) }) : t('accounts.neverSignedIn')}
                  </p>
                </div>
                <div className="flex flex-wrap gap-1">
                  <Button variant="ghost" onClick={() => setEditing(a)}>
                    {t('common.edit')}
                    <span className="sr-only">: {a.username}</span>
                  </Button>
                  {a.id !== selfId && (
                    <>
                      <Button variant="ghost" onClick={() => setResetting(a)}>
                        {t('accounts.resetPassword')}
                        <span className="sr-only">: {a.username}</span>
                      </Button>
                      <Button variant="ghost" onClick={() => setDeleting(a)}>
                        {t('common.delete')}
                        <span className="sr-only">: {a.username}</span>
                      </Button>
                    </>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      {creating && (
        <CreateDialog
          onClose={() => setCreating(false)}
          onCreated={(a) => {
            setCreating(false)
            setCreated(a)
            void queryClient.invalidateQueries({ queryKey: ['accounts'] })
          }}
        />
      )}
      {editing && (
        <EditDialog
          account={editing}
          selfId={selfId}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            void queryClient.invalidateQueries({ queryKey: ['accounts'] })
          }}
        />
      )}
      {resetting && (
        <ResetDialog
          account={resetting}
          onClose={() => setResetting(null)}
          onDone={() => {
            const a = resetting
            setResetting(null)
            setCreated(null)
            setPasswordReset(a)
            void queryClient.invalidateQueries({ queryKey: ['accounts'] })
          }}
        />
      )}
      <ConfirmDialog
        open={deleting !== null}
        title={t('accounts.deleteTitle', { name: deleting ? label(deleting) : '' })}
        message={t('accounts.deleteMessage')}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? errorText(remove.error, t) : null}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
        onClose={() => {
          setDeleting(null)
          remove.reset()
        }}
      />
    </div>
  )
}

function RoleChoice({ value, onChange }: { value: AccountRole; onChange: (r: AccountRole) => void }) {
  const { t } = useTranslation()
  return (
    <Choice<AccountRole>
      legend={t('accounts.role')}
      value={value}
      onChange={onChange}
      options={roles.map((r) => ({ value: r, label: t(`accounts.roles.${r}`), hint: t(`accounts.roleHint.${r}`) }))}
    />
  )
}

function CreateDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (a: Account) => void }) {
  const { t } = useTranslation()
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [role, setRole] = useState<AccountRole>('user')
  const [mismatch, setMismatch] = useState(false)
  const create = useMutation({
    mutationFn: () => createAccount({ username: username.trim(), displayName: displayName.trim(), password, role }),
    onSuccess: onCreated,
  })
  const { fields, form } = formErrors(create.error, t, (message) => t('common.saveFailed', { message }))
  const submit = () => {
    if (password !== confirm) {
      setMismatch(true)
      return
    }
    setMismatch(false)
    create.mutate()
  }
  return (
    <Dialog open onClose={onClose} title={t('accounts.new')}>
      <Form onSubmit={submit} onCancel={onClose} submitLabel={t('accounts.create')} busy={create.isPending} error={form}>
        <TextField
          label={t('accounts.username')}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          required
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          error={fields.username}
        />
        <TextField
          label={t('accounts.displayName')}
          autoComplete="off"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          error={fields.displayName}
        />
        <TextField
          label={t('accounts.newPassword')}
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
        <RoleChoice value={role} onChange={setRole} />
      </Form>
    </Dialog>
  )
}

function EditDialog({
  account,
  selfId,
  onClose,
  onSaved,
}: {
  account: Account
  selfId: number | undefined
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [displayName, setDisplayName] = useState(account.displayName)
  const [role, setRole] = useState<AccountRole>(account.role)
  const save = useMutation({
    mutationFn: () => updateAccount(account.id, { displayName: displayName.trim(), role }),
    onSuccess: () => {
      // Changing your own name or role changes what the header and menus show.
      if (account.id === selfId) void queryClient.invalidateQueries({ queryKey: ['auth'] })
      onSaved()
    },
  })
  const { fields, form } = formErrors(save.error, t, (message) => t('common.saveFailed', { message }))
  return (
    <Dialog open onClose={onClose} title={t('accounts.editTitle', { name: label(account) })}>
      <Form onSubmit={() => save.mutate()} onCancel={onClose} submitLabel={t('common.save')} busy={save.isPending} error={form}>
        <TextField
          label={t('accounts.displayName')}
          autoComplete="off"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          error={fields.displayName}
        />
        <RoleChoice value={role} onChange={setRole} />
      </Form>
    </Dialog>
  )
}

function ResetDialog({ account, onClose, onDone }: { account: Account; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [mismatch, setMismatch] = useState(false)
  const reset = useMutation({ mutationFn: () => resetPassword(account.id, password), onSuccess: onDone })
  const { fields, form } = formErrors(reset.error, t, (message) => t('common.saveFailed', { message }))
  const submit = () => {
    if (password !== confirm) {
      setMismatch(true)
      return
    }
    setMismatch(false)
    reset.mutate()
  }
  return (
    <Dialog open onClose={onClose} title={t('accounts.resetTitle', { name: label(account) })}>
      <p className="mb-4 text-slate-700 dark:text-slate-300">{t('accounts.resetPasswordText')}</p>
      <Form onSubmit={submit} onCancel={onClose} submitLabel={t('accounts.resetPassword')} busy={reset.isPending} error={form}>
        <TextField
          label={t('accounts.newPassword')}
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
      </Form>
    </Dialog>
  )
}
