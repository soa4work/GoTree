import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import { deleteHeirloom, getHeirloom } from '../api/endpoints'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { PageHeading } from '../components/PageHeading'
import { custodyPeriod } from '../heirlooms/custodyText'
import { HeirloomDialog } from '../heirlooms/HeirloomDialog'
import { fullName, lifespan } from '../lib/people'
import { Gallery } from '../media/Gallery'
import { UploadButton } from '../media/UploadButton'
import { CitationText } from '../sources/CitationEditor'
import { NotFound } from './NotFound'
import { formatDate } from '../lib/dates'
import { errorText } from '../lib/errors'

export function HeirloomPage() {
  const { t } = useTranslation()
  const id = Number(useParams().id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const heirloom = useQuery({ queryKey: ['heirlooms', id], queryFn: ({ signal }) => getHeirloom(id, signal) })
  const [editing, setEditing] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const remove = useMutation({
    mutationFn: () => deleteHeirloom(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['heirlooms'] })
      navigate('/heirlooms')
    },
  })

  if (heirloom.error instanceof ApiError && heirloom.error.status === 404) return <NotFound />
  if (heirloom.isError) return <p role="alert">{t('common.loadFailed')}</p>
  if (heirloom.isPending) return <p role="status">{t('app.loading')}</p>
  const h = heirloom.data
  const facts = [
    [t('heirloom.madeDate'), formatDate(h.madeDate)],
    [t('heirloom.origin'), h.originPlace?.fullName ?? ''],
    [t('heirloom.currentLocation'), h.currentLocation],
  ].filter(([, v]) => v)

  return (
    <article className="max-w-3xl space-y-8">
      <header className="space-y-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <PageHeading title={h.name}>{h.name}</PageHeading>
            <p className="mt-1 text-slate-700 dark:text-slate-300">{t(`heirloom.kinds.${h.kind}`)}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => setEditing(true)}>
              {t('common.edit')}
            </Button>
            <Button variant="ghost" onClick={() => setConfirmDelete(true)}>
              {t('heirloom.delete')}
            </Button>
          </div>
        </div>
        {h.description && <p className="whitespace-pre-line">{h.description}</p>}
        {facts.length > 0 && (
          <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[auto_1fr]">
            {facts.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="font-medium text-slate-700 dark:text-slate-300">{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
        )}
      </header>

      <Section title={t('heirloom.custody')}>
        {h.custody.length === 0 ? (
          <p className="text-slate-600 dark:text-slate-400">{t('heirloom.noCustody')}</p>
        ) : (
          <ol className="relative space-y-4 border-l-2 border-slate-300 pl-6 dark:border-slate-700">
            {h.custody.map((c) => (
              <li key={c.id} className="relative">
                <span aria-hidden="true" className="absolute top-1.5 -left-[1.95rem] size-3 rounded-full bg-brand-700 dark:bg-brand-100" />
                <p className="font-medium">
                  {c.person ? (
                    <Link to={`/people/${c.person.id}`} className="text-brand-700 underline dark:text-brand-100">
                      {fullName(c.person) ?? t('person.unknown')}
                    </Link>
                  ) : (
                    t('heirloom.notInTree')
                  )}
                  {c.person && lifespan(c.person) && (
                    <span className="ml-2 text-sm font-normal text-slate-600 dark:text-slate-400">{lifespan(c.person)}</span>
                  )}
                </p>
                <p className="text-sm text-slate-700 dark:text-slate-300">
                  {[custodyPeriod(t, c), t(`heirloom.ways.${c.how}`)].filter(Boolean).join(' · ')}
                </p>
                {c.notes && <p className="text-sm text-slate-600 dark:text-slate-400">{c.notes}</p>}
              </li>
            ))}
          </ol>
        )}
      </Section>

      <Section title={t('heirloom.photos')} actions={<UploadButton owner={{ entityType: 'heirloom', entityId: h.id }} />}>
        <Gallery owner={{ entityType: 'heirloom', entityId: h.id }} />
      </Section>

      {h.citations.length > 0 && (
        <Section title={t('citation.sources')}>
          <ul className="space-y-1">
            {h.citations.map((c) => (
              <li key={`${c.citationId}-${c.field}`}>
                <CitationText citation={c} />
              </li>
            ))}
          </ul>
        </Section>
      )}

      {h.notes && (
        <Section title={t('heirloom.notes')}>
          <p className="whitespace-pre-line">{h.notes}</p>
        </Section>
      )}

      {editing && <HeirloomDialog heirloom={h} onClose={() => setEditing(false)} />}
      <ConfirmDialog
        open={confirmDelete}
        title={t('heirloom.delete')}
        message={t('heirloom.deleteConfirm', { name: h.name })}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </article>
  )
}

function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  const id = useId()
  return (
    <section aria-labelledby={id} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-200 pb-2 dark:border-slate-800">
        <h2 id={id} className="text-xl font-semibold">
          {title}
        </h2>
        {actions}
      </div>
      {children}
    </section>
  )
}
