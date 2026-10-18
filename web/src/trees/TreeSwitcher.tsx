import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getAuthState } from '../api/client'
import { DialogShell } from '../components/DialogShell'
import { treeDisplayName } from '../lib/treeName'
import { TreePicker } from './TreePicker'

/** The tree's name in the header; it opens the list of the user's trees. */
export function TreeSwitcher() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  const tree = auth.data?.tree
  if (!tree) return null
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        aria-label={t('trees.switcherLabel', { name: treeDisplayName(tree.name, t) })}
        onClick={() => setOpen(true)}
        className="flex min-h-11 min-w-0 items-center gap-1 rounded-lg px-2 text-slate-700 hover:bg-slate-200 dark:text-slate-300 dark:hover:bg-slate-800"
      >
        <span className="max-w-[40vw] truncate">{treeDisplayName(tree.name, t)}</span>
        <svg viewBox="0 0 24 24" width={16} height={16} fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" className="shrink-0">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>
      <DialogShell open={open} onClose={() => setOpen(false)} title={t('trees.title')}>
        <div className="space-y-4">
          <TreePicker onDone={() => setOpen(false)} />
          {tree.role === 'owner' && (
            <Link to="/members" onClick={() => setOpen(false)} className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100">
              {t('members.manage')}
            </Link>
          )}
        </div>
      </DialogShell>
    </>
  )
}
