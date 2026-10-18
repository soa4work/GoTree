import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import { deleteSource, getSource } from '../api/endpoints'
import type { CitationLink } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { PageHeading } from '../components/PageHeading'
import { qualityKey } from '../sources/quality'
import { SourceDialog } from '../sources/SourceDialog'
import { NotFound } from './NotFound'
import { errorText } from '../lib/errors'

export function SourcePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const id = Number(useParams().id)
  const source = useQuery({ queryKey: ['source', id], queryFn: ({ signal }) => getSource(id, signal) })
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const remove = useMutation({
    mutationFn: () => deleteSource(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['sources'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      navigate('/sources')
    },
  })

  if (source.isError) {
    if (source.error instanceof ApiError && source.error.status === 404) return <NotFound />
    return <p role="alert">{t('common.loadFailed')}</p>
  }
  if (source.isPending) return <p role="status">{t('app.loading')}</p>
  const s = source.data

  const meta: [string, string][] = (
    [
      [t('source.author'), s.author],
      [t('source.publication'), s.publication],
      [t('source.repository'), s.repository?.name ?? ''],
      [t('source.callNumber'), s.callNumber],
    ] as [string, string][]
  ).filter(([, v]) => v)

  return (
    <article className="max-w-3xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <PageHeading title={s.title}>{s.title}</PageHeading>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => setEditing(true)}>
            {t('common.edit')}
          </Button>
          <Button variant="ghost" onClick={() => setDeleting(true)}>
            {t('source.delete')}
          </Button>
        </div>
      </div>
      {meta.length > 0 && (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
          {meta.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="font-medium text-slate-700 dark:text-slate-300">{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
      )}
      {s.notes && <p className="whitespace-pre-line">{s.notes}</p>}

      <section aria-labelledby="citations-heading" className="space-y-3">
        <h2 id="citations-heading" className="border-b border-slate-200 pb-2 text-xl font-semibold dark:border-slate-800">
          {t('source.citations')}
        </h2>
        {s.citations.length === 0 && <p className="text-slate-600 dark:text-slate-400">{t('source.noCitations')}</p>}
        <ul className="space-y-3">
          {s.citations.map((c) => (
            <li key={c.id} className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
              <p className="font-medium">
                {c.page || t('source.noPage')}
                {c.quality !== null && (
                  <span className="ml-2 text-sm font-normal text-slate-600 dark:text-slate-400">({t(qualityKey(c.quality))})</span>
                )}
              </p>
              {c.text && <blockquote className="mt-1 border-l-4 border-slate-300 pl-3 italic dark:border-slate-700">{c.text}</blockquote>}
              {c.links.length > 0 && (
                <>
                  <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">{t('source.supports')}</p>
                  <ul className="text-sm">
                    {c.links.map((l) => (
                      <li key={`${l.entityType}-${l.entityId}-${l.field}`}>
                        <LinkTarget link={l} />
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </li>
          ))}
        </ul>
      </section>

      <SourceDialog open={editing} onClose={() => setEditing(false)} source={s} onSaved={() => setEditing(false)} />
      <ConfirmDialog
        open={deleting}
        title={t('source.delete')}
        message={t('source.deleteConfirm', { title: s.title, count: s.citationCount })}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setDeleting(false)}
      />
    </article>
  )
}

/** "Birth of Anna Müller", linked to her page. */
function LinkTarget({ link }: { link: CitationLink }) {
  const { t } = useTranslation()
  let text: string
  if (link.entityType === 'event') {
    const [type = '', name = ''] = link.label.split('|')
    const [tag = '', custom] = type.split(':')
    const what = custom || t(`eventType.${tag}`, { defaultValue: tag })
    text = t('citation.eventOf', { event: what, name: name || t('person.unknown') })
  } else {
    const name = link.label || t('person.unknown')
    text = t(`citation.target.${link.entityType}`, { name })
  }
  if (link.field) text += ` (${t('citation.fieldNote', { field: link.field })})`
  return link.personId !== null ? (
    <Link to={`/people/${link.personId}`} className="text-brand-700 underline underline-offset-4 dark:text-brand-100">
      {text}
    </Link>
  ) : (
    <span>{text}</span>
  )
}
