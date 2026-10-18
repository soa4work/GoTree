import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { exportUrl, verifyExport, type ExportVersion, type Privacy } from '../api/endpoints'
import { getAuthState } from '../api/client'
import { Button } from '../components/Button'
import { Choice } from '../components/Choice'
import { errorText } from '../lib/errors'
import { dropReasonText } from './reportText'

export function ExportSection() {
  const { t } = useTranslation()
  const [version, setVersion] = useState<ExportVersion>('5.5.1')
  const [chosenPrivacy, setPrivacy] = useState<Privacy>('all')
  const [format, setFormat] = useState<'ged' | 'gedzip'>('ged')
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  // The check builds a whole database, so the server only lets editors start it.
  const canVerify = auth.data?.tree?.role !== 'viewer'
  // Viewers may not take a full copy of living people; the server refuses it.
  const viewer = !canVerify
  const privacy: Privacy = viewer && chosenPrivacy === 'all' ? 'exclude-living' : chosenPrivacy
  const verify = useMutation({ mutationFn: () => verifyExport(version) })

  return (
    <section aria-labelledby="export-heading" className="space-y-5">
      <h2 id="export-heading" className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
        {t('transfer.exportTitle')}
      </h2>
      <p className="text-slate-700 dark:text-slate-300">{t('transfer.exportIntro')}</p>
      <Choice
        legend={t('transfer.version')}
        value={version}
        onChange={setVersion}
        options={[
          { value: '5.5.1', label: 'GEDCOM 5.5.1', hint: t('transfer.v551Hint') },
          { value: '7.0', label: 'GEDCOM 7.0', hint: t('transfer.v7Hint') },
        ]}
      />
      <Choice
        legend={t('transfer.privacy')}
        value={privacy}
        onChange={setPrivacy}
        options={[
          ...(viewer ? [] : [{ value: 'all' as const, label: t('transfer.privacyAll'), hint: t('transfer.privacyAllHint') }]),
          { value: 'name-only', label: t('transfer.privacyNameOnly'), hint: t('transfer.privacyNameOnlyHint') },
          { value: 'exclude-living', label: t('transfer.privacyExclude'), hint: t('transfer.privacyExcludeHint') },
        ]}
      />
      {viewer && <p className="text-sm text-slate-600 dark:text-slate-400">{t('transfer.viewerPrivacy')}</p>}
      <Choice
        legend={t('transfer.format')}
        value={format}
        onChange={setFormat}
        options={[
          { value: 'ged', label: t('transfer.formatGed'), hint: t('transfer.formatGedHint') },
          { value: 'gedzip', label: t('transfer.formatZip'), hint: t('transfer.formatZipHint') },
        ]}
      />
      <div className="flex flex-wrap gap-3">
        <a
          href={exportUrl(version, privacy, format)}
          download
          className="inline-flex min-h-11 items-center rounded-lg bg-brand-700 px-4 font-medium text-white hover:bg-brand-800"
        >
          {t('transfer.download')}
        </a>
        {canVerify && (
          <Button variant="secondary" busy={verify.isPending} onClick={() => verify.mutate()}>
            {t('transfer.check')}
          </Button>
        )}
      </div>
      <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {verify.isPending ? t('transfer.checking') : ''}
      </p>
      {verify.error && (
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {t('transfer.checkFailed', { message: errorText(verify.error, t) })}
        </p>
      )}
      {verify.data && (
        <CheckResult ok={verify.data.ok} version={verify.data.version}>
          <table className="w-full text-left text-sm">
            <caption className="sr-only">{t('transfer.checkCaption')}</caption>
            <thead>
              <tr>
                <th scope="col" className="py-1">
                  {t('transfer.kind')}
                </th>
                <th scope="col" className="py-1 text-right">
                  {t('transfer.inTree')}
                </th>
                <th scope="col" className="py-1 text-right">
                  {t('transfer.afterRoundTrip')}
                </th>
              </tr>
            </thead>
            <tbody>
              {verify.data.rows.map((row) => (
                <tr key={row.kind} className="border-t border-slate-200 dark:border-slate-800">
                  <th scope="row" className="py-1 font-normal">
                    {t(`transfer.kinds.${row.kind}`, { defaultValue: row.kind })}
                  </th>
                  <td className="py-1 text-right">{row.tree}</td>
                  <td className={['py-1 text-right', row.tree !== row.roundTrip ? 'font-bold text-red-700 dark:text-red-400' : ''].join(' ')}>
                    {row.roundTrip}
                    {row.tree !== row.roundTrip && <span className="sr-only"> ({t('transfer.differs')})</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {verify.data.dropped.length > 0 && (
            <ul className="mt-2 list-disc pl-5 text-sm">
              {verify.data.dropped.map((d) => (
                <li key={d.path}>
                  {d.path} ({d.count}): {dropReasonText(t, d)}
                </li>
              ))}
            </ul>
          )}
        </CheckResult>
      )}
    </section>
  )
}

function CheckResult({ ok, version, children }: { ok: boolean; version: string; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className={['space-y-2 rounded-xl border p-4', ok ? 'border-leaf-500' : 'border-red-700 dark:border-red-400'].join(' ')}>
      <p className="font-semibold">{ok ? t('transfer.checkOk', { version }) : t('transfer.checkLoss', { version })}</p>
      {children}
    </div>
  )
}
