import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { FormError } from '../auth/FormError'
import { createTree, listTrees } from '../api/endpoints'
import { Button } from '../components/Button'
import { TextField } from '../components/TextField'
import { errorText, fieldErrors } from '../lib/errors'
import { useSwitchTree } from './useSwitchTree'

/**
 * The user's trees to switch to, and a form for a new one. The switcher's
 * dialog and the screen for users without a tree share it.
 */
export function TreePicker({ onDone }: { onDone?: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const trees = useQuery({ queryKey: ['trees'], queryFn: ({ signal }) => listTrees(signal) })
  const switcher = useSwitchTree()
  const create = useMutation({
    mutationFn: () => createTree(name.trim()),
    onSuccess: (tree) => {
      setName('')
      void queryClient.invalidateQueries({ queryKey: ['trees'] })
      switcher.mutate(tree.id, { onSuccess: onDone })
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    switcher.reset()
    create.mutate()
  }
  const fields = fieldErrors(create.error, t)
  const createError = create.error && Object.keys(fields).length === 0 ? t('common.saveFailed', { message: errorText(create.error, t) }) : null
  const switchError = switcher.error ? t('trees.switchFailed', { message: errorText(switcher.error, t) }) : null

  return (
    <div className="space-y-6">
      {trees.isPending && (
        <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
          {t('app.loading')}
        </p>
      )}
      {trees.isError && (
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {t('common.loadFailed')}
        </p>
      )}
      {trees.data && (
        <ul className="space-y-2">
          {trees.data.map((tree) => (
            <li key={tree.id}>
              <button
                type="button"
                aria-current={tree.current ? 'true' : undefined}
                disabled={switcher.isPending}
                onClick={() => (tree.current ? onDone?.() : switcher.mutate(tree.id, { onSuccess: onDone }))}
                className={[
                  'flex min-h-11 w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left disabled:opacity-60',
                  tree.current
                    ? 'border-brand-700 bg-brand-50 dark:border-brand-100 dark:bg-slate-800'
                    : 'border-slate-400 hover:bg-slate-100 dark:border-slate-600 dark:hover:bg-slate-800',
                ].join(' ')}
              >
                <span className="min-w-0 break-words font-medium">{tree.name}</span>
                <span className="flex shrink-0 items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
                  {tree.current && (
                    <span className="rounded bg-slate-200 px-1.5 py-0.5 text-xs font-semibold text-slate-800 dark:bg-slate-700 dark:text-slate-100">
                      {t('trees.current')}
                    </span>
                  )}
                  {t(`roles.${tree.role}`)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {switchError && (
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {switchError}
        </p>
      )}
      <form onSubmit={submit} noValidate className="space-y-3">
        <FormError message={createError} />
        <TextField
          label={t('trees.newName')}
          autoComplete="off"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={fields.name}
        />
        <Button type="submit" variant="secondary" busy={create.isPending}>
          {t('trees.create')}
        </Button>
      </form>
    </div>
  )
}
