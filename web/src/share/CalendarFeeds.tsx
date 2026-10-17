import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { createCalendarFeed, listCalendarFeeds, revokeCalendarFeed } from '../api/endpoints'
import type { CalendarFeed } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { formatDay } from '../lib/dates'

const feedUrl = (token: string) => `${window.location.origin}/ical/${token}.ics`

/** Secret calendar subscriptions for birthdays and anniversaries. */
export function CalendarFeeds() {
  const { t } = useTranslation()
  const id = useId()
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState<CalendarFeed | null>(null)
  const [revoking, setRevoking] = useState<CalendarFeed | null>(null)
  const feeds = useQuery({ queryKey: ['calendar-feeds'], queryFn: ({ signal }) => listCalendarFeeds(signal) })
  const revoke = useMutation({
    mutationFn: (feedId: number) => revokeCalendarFeed(feedId),
    onSuccess: () => {
      setRevoking(null)
      void queryClient.invalidateQueries({ queryKey: ['calendar-feeds'] })
    },
  })

  return (
    <section aria-labelledby={id} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-2 dark:border-slate-800">
        <h2 id={id} className="text-xl font-semibold">
          {t('calendar.title')}
        </h2>
        <Button variant="secondary" onClick={() => setCreating(true)}>
          + {t('calendar.new')}
        </Button>
      </div>
      <p className="text-slate-700 dark:text-slate-300">{t('calendar.intro')}</p>
      {created?.token && <CreatedFeed feed={created} onDone={() => setCreated(null)} />}
      {feeds.data && feeds.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
          {feeds.data.map((f) => (
            <li key={f.id} className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <p className="font-medium">
                  {f.label}
                  {f.revokedAt && (
                    <span className="ml-2 rounded bg-slate-200 px-1.5 py-0.5 text-xs font-semibold dark:bg-slate-800">
                      {t('sharing.revoked')}
                    </span>
                  )}
                </p>
                <p className="text-sm text-slate-600 dark:text-slate-400">
                  {[
                    f.includeLiving ? t('calendar.withLiving') : t('calendar.deceasedOnly'),
                    f.lastUsedAt ? t('sharing.lastUsed', { date: formatDay(f.lastUsedAt) }) : t('sharing.neverUsed'),
                  ].join(' · ')}
                </p>
              </div>
              {!f.revokedAt && (
                <Button variant="ghost" onClick={() => setRevoking(f)}>
                  {t('sharing.revoke')}
                  <span className="sr-only">: {f.label}</span>
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      {creating && (
        <CreateFeedDialog
          onClose={() => setCreating(false)}
          onCreated={(f) => {
            setCreating(false)
            setCreated(f)
            void queryClient.invalidateQueries({ queryKey: ['calendar-feeds'] })
          }}
        />
      )}
      <ConfirmDialog
        open={revoking !== null}
        title={t('calendar.revokeTitle')}
        message={t('calendar.revokeConfirm', { label: revoking?.label ?? '' })}
        confirmLabel={t('sharing.revoke')}
        busy={revoke.isPending}
        error={revoke.error ? t('common.saveFailed', { message: revoke.error.message }) : null}
        onConfirm={() => revoking && revoke.mutate(revoking.id)}
        onClose={() => setRevoking(null)}
      />
    </section>
  )
}

function CreatedFeed({ feed, onDone }: { feed: CalendarFeed; onDone: () => void }) {
  const { t } = useTranslation()
  const id = useId()
  const [copied, setCopied] = useState(false)
  const url = feedUrl(feed.token!)
  const webcal = url.replace(/^https?:/, 'webcal:')
  return (
    <div className="space-y-3 rounded-xl border-2 border-brand-500 p-4">
      <p className="font-semibold">{t('calendar.createdTitle', { label: feed.label })}</p>
      <p>{t('calendar.createdHint')}</p>
      <label htmlFor={id} className="block text-sm font-medium">
        {t('calendar.urlLabel')}
      </label>
      <input
        id={id}
        readOnly
        value={url}
        onFocus={(e) => e.target.select()}
        className="block min-h-11 w-full rounded-lg border border-slate-400 bg-white px-3 font-mono text-sm dark:border-slate-600 dark:bg-slate-900"
      />
      <div className="flex flex-wrap gap-2">
        <Button onClick={() => void navigator.clipboard?.writeText(url).then(() => setCopied(true))}>
          {t('sharing.copy')}
        </Button>
        <a
          href={webcal}
          className="inline-flex min-h-11 items-center rounded-lg border border-slate-300 px-4 font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900"
        >
          {t('calendar.subscribe')}
        </a>
        <Button variant="secondary" onClick={onDone}>
          {t('sharing.done')}
        </Button>
      </div>
      <p role="status" className="text-sm font-medium text-green-800 dark:text-green-300">
        {copied ? t('sharing.copied') : ''}
      </p>
    </div>
  )
}

function CreateFeedDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (f: CalendarFeed) => void }) {
  const { t } = useTranslation()
  const livingId = useId()
  const [label, setLabel] = useState('')
  const [includeLiving, setIncludeLiving] = useState(false)
  const create = useMutation({ mutationFn: () => createCalendarFeed({ label, includeLiving }), onSuccess: onCreated })
  const fields = create.error instanceof ApiError ? create.error.fields : {}
  return (
    <Dialog open onClose={onClose} title={t('calendar.new')}>
      <Form
        onSubmit={() => create.mutate()}
        onCancel={onClose}
        submitLabel={t('calendar.create')}
        busy={create.isPending}
        error={
          create.error && Object.keys(fields).length === 0
            ? t('common.saveFailed', { message: create.error.message })
            : null
        }
      >
        <TextField
          label={t('sharing.label')}
          hint={t('calendar.labelHint')}
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          error={fields.label}
        />
        <div className="flex items-start gap-3">
          <input
            id={livingId}
            type="checkbox"
            checked={includeLiving}
            onChange={(e) => setIncludeLiving(e.target.checked)}
            aria-describedby={`${livingId}-hint`}
            className="mt-1 size-5 shrink-0"
          />
          <div>
            <label htmlFor={livingId} className="block font-medium">
              {t('calendar.includeLiving')}
            </label>
            <p id={`${livingId}-hint`} className="text-sm text-slate-600 dark:text-slate-400">
              {t('calendar.includeLivingHint')}
            </p>
          </div>
        </div>
      </Form>
    </Dialog>
  )
}
