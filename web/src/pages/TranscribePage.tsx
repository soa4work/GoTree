import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { listTemplates, listTranscriptions } from '../api/endpoints'
import type { RecordTemplate } from '../api/types'
import { Button } from '../components/Button'
import { PageHeading } from '../components/PageHeading'
import { templateName } from '../transcribe/labels'
import { TemplateDialog } from '../transcribe/TemplateDialog'
import { formatDate } from '../lib/dates'

export function TranscribePage() {
  const { t } = useTranslation()
  const listId = useId()
  const templatesId = useId()
  const [editing, setEditing] = useState<RecordTemplate | 'new' | null>(null)
  const templates = useQuery({ queryKey: ['templates'], queryFn: ({ signal }) => listTemplates(signal) })
  const list = useQuery({ queryKey: ['transcriptions', 'list'], queryFn: ({ signal }) => listTranscriptions(signal) })
  const byKey = new Map((templates.data ?? []).map((x) => [x.key, x]))
  const custom = (templates.data ?? []).filter((x) => !x.builtin)

  return (
    <div className="max-w-3xl space-y-8">
      <div className="space-y-3">
        <PageHeading title={t('nav.transcribe')}>{t('nav.transcribe')}</PageHeading>
        <p className="text-slate-700 dark:text-slate-300">{t('transcribe.intro')}</p>
      </div>

      <nav aria-label={t('transcribe.new')}>
        <h2 className="text-xl font-semibold">{t('transcribe.new')}</h2>
        <ul className="mt-2 grid gap-2 sm:grid-cols-2">
          {(templates.data ?? []).map((x) => (
            <li key={x.key}>
              <Link
                to={`/transcribe/new?template=${encodeURIComponent(x.key)}`}
                className="flex min-h-11 items-center rounded-lg border border-slate-200 px-4 py-2 font-medium hover:bg-slate-50 dark:border-slate-800 dark:hover:bg-slate-900"
              >
                + {templateName(t, x)}
              </Link>
            </li>
          ))}
        </ul>
      </nav>

      <section aria-labelledby={listId} className="space-y-3">
        <h2 id={listId} className="text-xl font-semibold">
          {t('transcribe.list')}
        </h2>
        <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
          {list.isPending
            ? t('app.loading')
            : list.isError
              ? t('common.loadFailed')
              : list.data.length === 0
                ? t('transcribe.none')
                : t('transcribe.count', { count: list.data.length })}
        </p>
        {list.data && list.data.length > 0 && (
          <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
            {list.data.map((tr) => {
              const tpl = byKey.get(tr.templateKey)
              const kind = tpl ? templateName(t, tpl) : tr.templateKey
              return (
                <li key={tr.id}>
                  <Link to={`/transcribe/${tr.id}`} className="block min-h-11 px-4 py-2 hover:bg-slate-50 dark:hover:bg-slate-900">
                    <span className="block font-medium">{tr.title || t('transcribe.untitled', { template: kind })}</span>
                    <span className="block text-sm text-slate-600 dark:text-slate-400">
                      {[kind, tr.source?.title, formatDate(tr.date), t('transcribe.rowsCount', { count: tr.rows.length }), t(`transcribe.${tr.status}`)]
                        .filter(Boolean)
                        .join(' · ')}
                    </span>
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      <section aria-labelledby={templatesId} className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 id={templatesId} className="text-xl font-semibold">
            {t('transcribe.templates')}
          </h2>
          <Button variant="secondary" onClick={() => setEditing('new')}>
            + {t('transcribe.newTemplate')}
          </Button>
        </div>
        <p className="text-slate-700 dark:text-slate-300">{t('transcribe.templatesIntro')}</p>
        {custom.length > 0 && (
          <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
            {custom.map((x) => (
              <li key={x.key}>
                <button type="button" onClick={() => setEditing(x)} className="block min-h-11 w-full px-4 py-2 text-left hover:bg-slate-50 dark:hover:bg-slate-900">
                  <span className="block font-medium">{x.name}</span>
                  <span className="block text-sm text-slate-600 dark:text-slate-400">
                    {[t(`eventType.${x.eventType}`, { defaultValue: x.eventType }), x.roles.map((r) => r.label).join(', ')].join(' · ')}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      {editing && <TemplateDialog template={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} />}
    </div>
  )
}
