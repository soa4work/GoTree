import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, getAuthState } from '../api/client'
import { importGedcom, listPersons, listShareLinks } from '../api/endpoints'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { PageHeading } from '../components/PageHeading'
import { ExportSection } from '../transfer/ExportSection'
import { ImportReportView } from '../transfer/ImportReportView'
import { errorText } from '../lib/errors'

export function ImportExportPage() {
  const { t } = useTranslation()
  return (
    <div className="max-w-3xl space-y-8">
      <PageHeading title={t('nav.importExport')}>{t('nav.importExport')}</PageHeading>
      <ExportSection />
      <ImportSection />
      <BackupSection />
    </div>
  )
}

function BackupSection() {
  const { t } = useTranslation()
  const id = useId()
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  if (auth.data?.user?.role !== 'admin') return null
  return (
    <section aria-labelledby={id} className="space-y-3">
      <h2 id={id} className="text-xl font-semibold">
        {t('backup.title')}
      </h2>
      <p className="text-slate-700 dark:text-slate-300">{t('backup.intro')}</p>
      <a
        href="/api/backup"
        download
        className="inline-flex min-h-11 items-center rounded-lg bg-brand-700 px-4 font-medium text-white hover:bg-brand-800"
      >
        {t('backup.download')}
      </a>
    </section>
  )
}

function ImportSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileId = useId()
  const [file, setFile] = useState<File | null>(null)
  const [confirmReplace, setConfirmReplace] = useState(false)
  const people = useQuery({ queryKey: ['persons', 'count'], queryFn: ({ signal }) => listPersons('', 1, 0, signal) })
  const hasData = (people.data?.total ?? 0) > 0
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  // Replacing wipes the tree: only its owner may do it.
  const isOwner = auth.data?.tree?.role === 'owner'
  const replaceBlocked = hasData && !isOwner
  // Share links to a branch may be deleted by replacing; the list is
  // owner-only on the server, like replacing itself.
  const shares = useQuery({ queryKey: ['share-links'], queryFn: ({ signal }) => listShareLinks(signal), enabled: hasData && isOwner })
  const now = new Date().toISOString()
  const branchLinks = (shares.data ?? []).filter(
    (l) => l.scope === 'descendants' && !l.revokedAt && (!l.expiresAt || l.expiresAt > now),
  ).length
  const replaceConfirm = t('transfer.replaceConfirm', { count: people.data?.total ?? 0 })

  const run = useMutation({
    mutationFn: (mode: 'empty' | 'replace') => importGedcom(file!, mode),
    onSuccess: () => {
      setConfirmReplace(false)
      void queryClient.invalidateQueries()
    },
  })
  const error =
    run.error instanceof ApiError && run.error.status === 409
      ? t('transfer.notEmpty')
      : run.error
        ? t('transfer.importFailed', { message: errorText(run.error, t) })
        : null

  return (
    <section aria-labelledby="import-heading" className="space-y-4">
      <h2 id="import-heading" className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
        {t('transfer.importTitle')}
      </h2>
      <p className="text-slate-700 dark:text-slate-300">{t('transfer.importIntro')}</p>
      <div>
        <label htmlFor={fileId} className="block text-sm font-medium">
          {t('transfer.file')}
        </label>
        <input
          id={fileId}
          type="file"
          accept=".ged,.zip,.gdz"
          onChange={(e) => {
            setFile(e.target.files?.[0] ?? null)
            run.reset()
          }}
          className="mt-1 block w-full text-base file:mr-3 file:min-h-11 file:rounded-lg file:border file:border-slate-400 file:bg-white file:px-4 file:font-medium dark:file:border-slate-600 dark:file:bg-slate-900 dark:file:text-slate-100"
        />
        <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">{t('transfer.fileHint')}</p>
      </div>
      {hasData && <p className="font-medium text-amber-800 dark:text-amber-300">{t('transfer.replaceWarning')}</p>}
      {hasData && branchLinks > 0 && (
        <p className="text-slate-700 dark:text-slate-300">{t('transfer.replaceShareLinks', { count: branchLinks })}</p>
      )}
      {replaceBlocked && <p className="text-slate-700 dark:text-slate-300">{t('transfer.ownerOnly')}</p>}
      <Button
        disabled={!file || replaceBlocked}
        busy={run.isPending && !confirmReplace}
        onClick={() => (hasData ? setConfirmReplace(true) : run.mutate('empty'))}
      >
        {hasData ? t('transfer.replaceButton') : t('transfer.importButton')}
      </Button>
      <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {run.isPending ? t('transfer.importing') : ''}
      </p>
      {error && (
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {error}
        </p>
      )}
      {run.data && <ImportReportView report={run.data} />}
      <ConfirmDialog
        open={confirmReplace}
        title={t('transfer.replaceTitle')}
        message={branchLinks > 0 ? `${replaceConfirm} ${t('transfer.replaceShareLinks', { count: branchLinks })}` : replaceConfirm}
        confirmLabel={t('transfer.replaceButton')}
        busy={run.isPending}
        error={error}
        onConfirm={() => run.mutate('replace')}
        onClose={() => setConfirmReplace(false)}
      />
    </section>
  )
}
