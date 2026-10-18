import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import type { ImportReport, TagStat } from '../api/types'

const countKeys = ['persons', 'families', 'events', 'places', 'sources', 'repositories', 'citations', 'media'] as const

export function ImportReportView({ report }: { report: ImportReport }) {
  const { t } = useTranslation()
  const byOutcome = (o: TagStat['outcome']) => report.tags.filter((x) => x.outcome === o)
  const kept = byOutcome('kept')
  const dropped = byOutcome('dropped')

  return (
    <section aria-labelledby="import-report" className="space-y-4 rounded-xl border border-leaf-500 p-4">
      <h3 id="import-report" className="text-lg font-semibold">
        {t('transfer.reportTitle')}
      </h3>
      <p>
        {t('transfer.reportFrom', {
          source: report.source || t('transfer.unknownProgram'),
          version: report.version || '?',
          encoding: report.encoding,
          seconds: (report.durationMs / 1000).toFixed(1),
        })}
      </p>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-1 sm:grid-cols-4">
        {countKeys.map((k) => (
          <div key={k}>
            <dt className="text-sm text-slate-600 dark:text-slate-400">{t(`transfer.counts.${k}`)}</dt>
            <dd className="text-xl font-semibold">{report.counts[k] ?? 0}</dd>
          </div>
        ))}
      </dl>
      <p>
        <Link to="/people" className="font-medium text-brand-700 underline underline-offset-4 dark:text-brand-100">
          {t('transfer.viewPeople')}
        </Link>
      </p>

      {report.invalidDates > 0 && (
        <Problem title={t('transfer.invalidDates', { count: report.invalidDates })} hint={t('transfer.invalidDatesHint')}>
          {report.invalidDateExamples}
        </Problem>
      )}
      {report.brokenReferences.length > 0 && (
        <Problem title={t('transfer.brokenRefs', { count: report.brokenReferences.length })} hint={t('transfer.brokenRefsHint')}>
          {report.brokenReferences}
        </Problem>
      )}
      {report.childrenFromFamc > 0 && (
        <Problem title={t('transfer.famcChildren', { count: report.childrenFromFamc })} hint={t('transfer.famcChildrenHint')}>
          {report.childrenFromFamcExamples}
        </Problem>
      )}
      {(report.shareLinksLost?.length ?? 0) > 0 && (
        <Problem title={t('transfer.shareLinksLost', { count: report.shareLinksLost.length })} hint={t('transfer.shareLinksLostHint')}>
          {report.shareLinksLost}
        </Problem>
      )}
      {(report.shareLinksKept ?? 0) > 0 && <p>{t('transfer.shareLinksKept', { count: report.shareLinksKept })}</p>}
      {report.warnings.length > 0 && (
        <Problem title={t('transfer.warnings', { count: report.warnings.length })}>
          {report.warnings.map((w) => (w.line ? t('transfer.atLine', { line: w.line, message: w.message }) : w.message))}
        </Problem>
      )}

      <details className="rounded-lg border border-slate-200 p-3 dark:border-slate-800">
        <summary className="cursor-pointer font-medium">
          {t('transfer.tagsSummary', { kept: kept.length, dropped: dropped.length })}
        </summary>
        <p className="mt-2 text-sm text-slate-700 dark:text-slate-300">{t('transfer.tagsHint')}</p>
        <TagTable title={t('transfer.keptTitle')} tags={kept} />
        <TagTable title={t('transfer.droppedTitle')} tags={dropped} />
        <TagTable title={t('transfer.mappedTitle')} tags={byOutcome('mapped')} />
      </details>
    </section>
  )
}

function Problem({ title, hint, children }: { title: string; hint?: string; children: string[] }) {
  return (
    <div className="rounded-lg border border-amber-500 bg-amber-50 p-3 dark:bg-amber-950/40">
      <p className="font-medium">{title}</p>
      {hint && <p className="text-sm text-slate-700 dark:text-slate-300">{hint}</p>}
      <ul className="mt-1 list-disc pl-5 text-sm">
        {children.slice(0, 20).map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </div>
  )
}

function TagTable({ title, tags }: { title: string; tags: TagStat[] }) {
  const { t } = useTranslation()
  if (tags.length === 0) return null
  return (
    <table className="mt-3 w-full text-left text-sm">
      <caption className="text-left font-semibold">{title}</caption>
      <thead>
        <tr>
          <th scope="col" className="py-1 pr-3">
            {t('transfer.tag')}
          </th>
          <th scope="col" className="py-1 pr-3 text-right">
            {t('transfer.count')}
          </th>
          <th scope="col" className="py-1">
            {t('transfer.reason')}
          </th>
        </tr>
      </thead>
      <tbody>
        {tags.map((tag) => (
          <tr key={`${tag.path}-${tag.outcome}`} className="border-t border-slate-200 dark:border-slate-800">
            <td className="py-1 pr-3 font-mono">{tag.path}</td>
            <td className="py-1 pr-3 text-right">{tag.count}</td>
            <td className="py-1">{tag.reason ?? ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
