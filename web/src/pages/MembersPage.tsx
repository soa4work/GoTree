import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { getAuthState, type TreeRole } from '../api/client'
import { addMember, listMembers, removeMember, renameTree, setMemberRole, type Member } from '../api/endpoints'
import { FormError } from '../auth/FormError'
import { Button } from '../components/Button'
import { Choice } from '../components/Choice'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { errorText, fieldErrors } from '../lib/errors'

const roles: TreeRole[] = ['owner', 'editor', 'viewer']

export function MembersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  const [leaving, setLeaving] = useState(false)
  const tree = auth.data?.tree
  const user = auth.data?.user
  const leave = useMutation({
    mutationFn: () => removeMember(tree!.id, user!.id),
    onSuccess: () => {
      setLeaving(false)
      // Same as switching away: nothing loaded from this tree may stay. The
      // auth refetch then shows the tree picker.
      queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== 'auth' })
      void queryClient.invalidateQueries({ queryKey: ['auth'] })
    },
  })
  if (!tree || !user) return null
  const owner = tree.role === 'owner'

  return (
    <div className="max-w-3xl space-y-6">
      <PageHeading title={t('nav.members')}>{t('nav.members')}</PageHeading>
      <p className="text-slate-700 dark:text-slate-300">{t('members.intro')}</p>
      {owner ? (
        <>
          <RenameSection treeId={tree.id} name={tree.name} />
          <MemberList treeId={tree.id} selfId={user.id} />
          <AddSection treeId={tree.id} />
        </>
      ) : (
        <div className="space-y-2">
          <p className="text-slate-700 dark:text-slate-300">{t('members.ownerOnly')}</p>
          <p className="font-medium">{t('members.yourRole', { role: t(`roles.${tree.role}`) })}</p>
        </div>
      )}
      <div>
        <Button variant="danger" onClick={() => setLeaving(true)}>
          {t('members.leave')}
        </Button>
      </div>
      <ConfirmDialog
        open={leaving}
        title={t('members.leaveTitle')}
        message={t('members.leaveMessage')}
        confirmLabel={t('members.leave')}
        busy={leave.isPending}
        error={leave.error ? errorText(leave.error, t) : null}
        onConfirm={() => leave.mutate()}
        onClose={() => {
          setLeaving(false)
          leave.reset()
        }}
      />
    </div>
  )
}

function RenameSection({ treeId, name: current }: { treeId: number; name: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState(current)
  const rename = useMutation({
    mutationFn: () => renameTree(treeId, name.trim()),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['auth'] })
      void queryClient.invalidateQueries({ queryKey: ['trees'] })
    },
  })
  const fields = fieldErrors(rename.error, t)
  const formError = rename.error && Object.keys(fields).length === 0 ? t('common.saveFailed', { message: errorText(rename.error, t) }) : null
  const submit = (e: FormEvent) => {
    e.preventDefault()
    rename.mutate()
  }
  return (
    <form onSubmit={submit} noValidate className="max-w-md space-y-3">
      <FormError message={formError} />
      <TextField label={t('members.treeName')} autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} error={fields.name} />
      <Button type="submit" variant="secondary" busy={rename.isPending}>
        {t('common.save')}
      </Button>
    </form>
  )
}

function MemberList({ treeId, selfId }: { treeId: number; selfId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [removing, setRemoving] = useState<Member | null>(null)
  const members = useQuery({ queryKey: ['members', treeId], queryFn: ({ signal }) => listMembers(treeId, signal) })
  const changed = (userId: number) => {
    void queryClient.invalidateQueries({ queryKey: ['members', treeId] })
    // Changing your own role changes what this page and the menus show.
    if (userId === selfId) {
      void queryClient.invalidateQueries({ queryKey: ['auth'] })
      void queryClient.invalidateQueries({ queryKey: ['trees'] })
    }
  }
  const setRole = useMutation({
    mutationFn: ({ userId, role }: { userId: number; role: TreeRole }) => setMemberRole(treeId, userId, role),
    onSuccess: (_m, v) => changed(v.userId),
  })
  const remove = useMutation({
    mutationFn: (userId: number) => removeMember(treeId, userId),
    onSuccess: (_r, userId) => {
      setRemoving(null)
      changed(userId)
    },
  })
  const list = members.data ?? []

  return (
    <section aria-label={t('nav.members')} className="space-y-3">
      {(members.isPending || members.isError) && (
        <p role={members.isError ? 'alert' : 'status'} className="text-sm text-slate-600 dark:text-slate-400">
          {members.isPending ? t('app.loading') : t('common.loadFailed')}
        </p>
      )}
      {members.data && (
        <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
          {t('members.count', { count: list.length })}
        </p>
      )}
      <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
        {list.map((m) => {
          const name = m.displayName || m.username
          const rowError = setRole.error && setRole.variables?.userId === m.userId ? errorText(setRole.error, t) : null
          return (
            <li key={m.userId} className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <p className="break-words font-medium">{name}</p>
                <p className="text-sm text-slate-600 dark:text-slate-400">@{m.username}</p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <RoleSelect
                  name={name}
                  value={m.role}
                  disabled={setRole.isPending && setRole.variables?.userId === m.userId}
                  onChange={(role) => setRole.mutate({ userId: m.userId, role })}
                />
                {m.userId !== selfId && (
                  <Button variant="ghost" onClick={() => setRemoving(m)}>
                    {t('common.remove')}
                    <span className="sr-only">: {name}</span>
                  </Button>
                )}
              </div>
              {rowError && (
                <p role="alert" className="w-full text-sm font-medium text-red-700 dark:text-red-400">
                  {rowError}
                </p>
              )}
            </li>
          )
        })}
      </ul>
      <ConfirmDialog
        open={removing !== null}
        title={t('members.removeTitle', { name: removing ? removing.displayName || removing.username : '' })}
        message={t('members.removeMessage', { name: removing ? removing.displayName || removing.username : '' })}
        confirmLabel={t('members.remove')}
        busy={remove.isPending}
        error={remove.error ? errorText(remove.error, t) : null}
        onConfirm={() => removing && remove.mutate(removing.userId)}
        onClose={() => {
          setRemoving(null)
          remove.reset()
        }}
      />
    </section>
  )
}

function RoleSelect({ name, value, disabled, onChange }: { name: string; value: TreeRole; disabled: boolean; onChange: (r: TreeRole) => void }) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="sr-only">
        {t('members.roleFor', { name })}
      </label>
      <select
        id={id}
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value as TreeRole)}
        className="min-h-11 rounded-lg border border-slate-400 bg-white px-3 text-base text-slate-900 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100"
      >
        {roles.map((r) => (
          <option key={r} value={r}>
            {t(`roles.${r}`)}
          </option>
        ))}
      </select>
    </div>
  )
}

function AddSection({ treeId }: { treeId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const headingId = useId()
  const [username, setUsername] = useState('')
  const [role, setRole] = useState<TreeRole>('viewer')
  const add = useMutation({
    mutationFn: () => addMember(treeId, username.trim(), role),
    onSuccess: () => {
      setUsername('')
      setRole('viewer')
      void queryClient.invalidateQueries({ queryKey: ['members', treeId] })
    },
  })
  const fields = fieldErrors(add.error, t)
  const formError = add.error && Object.keys(fields).length === 0 ? t('common.saveFailed', { message: errorText(add.error, t) }) : null
  const submit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate()
  }
  return (
    <section aria-labelledby={headingId} className="space-y-3">
      <h2 id={headingId} className="text-lg font-semibold">
        {t('members.add')}
      </h2>
      <form onSubmit={submit} noValidate className="max-w-md space-y-4">
        <FormError message={formError} />
        <TextField
          label={t('members.username')}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          error={fields.username ?? fields.role}
        />
        <Choice
          legend={t('members.role')}
          value={role}
          onChange={setRole}
          options={roles.map((r) => ({ value: r, label: t(`roles.${r}`), hint: t(`members.roleHint.${r}`) }))}
        />
        <Button type="submit" busy={add.isPending}>
          {t('members.add')}
        </Button>
        <p role="status">{add.isSuccess ? t('members.added', { name: add.data.displayName || add.data.username }) : ''}</p>
      </form>
    </section>
  )
}
