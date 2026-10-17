import type { TFunction } from 'i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { createShareLink, listShareLinks, revokeShareLink } from '../api/endpoints'
import type { PersonRef, ShareLink, SharePrivacy, ShareScope } from '../api/types'
import { Button } from '../components/Button'
import { Choice } from '../components/Choice'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { Form } from '../components/Form'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { fullName } from '../lib/people'
import { PersonPicker } from '../people/PersonPicker'
import { CalendarFeeds } from '../share/CalendarFeeds'
import { formatDay } from '../lib/dates'

const shareUrl = (token: string) => `${window.location.origin}/share/${token}`

export function SharingPage() {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState<ShareLink | null>(null)
  const [revoking, setRevoking] = useState<ShareLink | null>(null)
  const queryClient = useQueryClient()
  const links = useQuery({ queryKey: ['share-links'], queryFn: ({ signal }) => listShareLinks(signal) })
  const revoke = useMutation({
    mutationFn: (id: number) => revokeShareLink(id),
    onSuccess: () => {
      setRevoking(null)
      void queryClient.invalidateQueries({ queryKey: ['share-links'] })
    },
  })
  const ownerOnly = links.error instanceof ApiError && links.error.status === 403

  return (
    <div className="max-w-3xl space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeading title={t('nav.sharing')}>{t('nav.sharing')}</PageHeading>
        {!ownerOnly && <Button onClick={() => setCreating(true)}>+ {t('sharing.new')}</Button>}
      </div>
      <p className="text-slate-700 dark:text-slate-300">{t('sharing.intro')}</p>

      {created?.token && <CreatedLink link={created} onDone={() => setCreated(null)} />}

      <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {links.isPending
          ? t('app.loading')
          : ownerOnly
            ? t('sharing.ownerOnly')
            : links.isError
              ? t('common.loadFailed')
              : t('sharing.count', { count: links.data.length })}
      </p>
      {links.data && links.data.length > 0 && (
        <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
          {links.data.map((l) => (
            <li key={l.id} className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <p className="font-medium">
                  {l.label}
                  {l.revokedAt && (
                    <span className="ml-2 rounded bg-slate-200 px-1.5 py-0.5 text-xs font-semibold dark:bg-slate-800">
                      {t('sharing.revoked')}
                    </span>
                  )}
                </p>
                <p className="text-sm text-slate-600 dark:text-slate-400">{describe(t, l)}</p>
              </div>
              {!l.revokedAt && (
                <Button variant="ghost" onClick={() => setRevoking(l)}>
                  {t('sharing.revoke')}
                  <span className="sr-only">: {l.label}</span>
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}

      {links.isSuccess && <CalendarFeeds />}

      {creating && (
        <CreateDialog
          onClose={() => setCreating(false)}
          onCreated={(l) => {
            setCreating(false)
            setCreated(l)
            void queryClient.invalidateQueries({ queryKey: ['share-links'] })
          }}
        />
      )}
      <ConfirmDialog
        open={revoking !== null}
        title={t('sharing.revokeTitle')}
        message={t('sharing.revokeConfirm', { label: revoking?.label ?? '' })}
        confirmLabel={t('sharing.revoke')}
        busy={revoke.isPending}
        error={revoke.error ? t('common.saveFailed', { message: revoke.error.message }) : null}
        onConfirm={() => revoking && revoke.mutate(revoking.id)}
        onClose={() => setRevoking(null)}
      />
    </div>
  )
}

function describe(t: TFunction, l: ShareLink): string {
  const scope =
    l.scope === 'tree'
      ? t('sharing.scopeTree')
      : t('sharing.scopeDescendantsOf', { name: (l.root && fullName(l.root)) ?? t('person.unknown') })
  return [
    scope,
    t(`sharing.privacy_${l.privacy}`),
    l.revokedAt
      ? t('sharing.revokedOn', { date: formatDay(l.revokedAt) })
      : l.expiresAt
        ? t('sharing.expires', { date: formatDay(new Date(new Date(l.expiresAt).getTime() - 1).toISOString()) })
        : t('sharing.noExpiry'),
    l.lastUsedAt ? t('sharing.lastUsed', { date: formatDay(l.lastUsedAt) }) : t('sharing.neverUsed'),
  ].join(' · ')
}

function CreatedLink({ link, onDone }: { link: ShareLink; onDone: () => void }) {
  const { t } = useTranslation()
  const id = useId()
  const [copied, setCopied] = useState(false)
  const url = shareUrl(link.token!)
  return (
    <section aria-labelledby={id} className="space-y-3 rounded-xl border-2 border-brand-500 p-4">
      <h2 id={id} className="text-lg font-semibold">
        {t('sharing.createdTitle', { label: link.label })}
      </h2>
      <p>{t('sharing.createdHint')}</p>
      <label htmlFor={`${id}-url`} className="sr-only">
        {t('sharing.linkLabel')}
      </label>
      <input
        id={`${id}-url`}
        readOnly
        value={url}
        onFocus={(e) => e.target.select()}
        className="block min-h-11 w-full rounded-lg border border-slate-400 bg-white px-3 font-mono text-sm dark:border-slate-600 dark:bg-slate-900"
      />
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={() => {
            void navigator.clipboard?.writeText(url).then(() => setCopied(true))
          }}
        >
          {t('sharing.copy')}
        </Button>
        <Button variant="secondary" onClick={onDone}>
          {t('sharing.done')}
        </Button>
      </div>
      <p role="status" className="text-sm font-medium text-green-800 dark:text-green-300">
        {copied ? t('sharing.copied') : ''}
      </p>
    </section>
  )
}

function CreateDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (l: ShareLink) => void }) {
  const { t } = useTranslation()
  const [label, setLabel] = useState('')
  const [scope, setScope] = useState<ShareScope>('tree')
  const [root, setRoot] = useState<PersonRef | null>(null)
  const [privacy, setPrivacy] = useState<SharePrivacy>('deceased')
  const [expiresOn, setExpiresOn] = useState('')
  const create = useMutation({
    mutationFn: () => createShareLink({ label, scope, rootPersonId: root?.id ?? null, privacy, expiresOn }),
    onSuccess: onCreated,
  })
  const fields = create.error instanceof ApiError ? create.error.fields : {}
  return (
    <Dialog open onClose={onClose} title={t('sharing.new')}>
      <Form
        onSubmit={() => create.mutate()}
        onCancel={onClose}
        submitLabel={t('sharing.create')}
        busy={create.isPending}
        error={
          create.error && Object.keys(fields).length === 0
            ? t('common.saveFailed', { message: create.error.message })
            : null
        }
      >
        <TextField
          label={t('sharing.label')}
          hint={t('sharing.labelHint')}
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          error={fields.label}
        />
        <Choice<ShareScope>
          legend={t('sharing.scope')}
          value={scope}
          onChange={setScope}
          options={[
            { value: 'tree', label: t('sharing.scopeTree'), hint: t('sharing.scopeTreeHint') },
            { value: 'descendants', label: t('sharing.scopeDescendants'), hint: t('sharing.scopeDescendantsHint') },
          ]}
        />
        {scope === 'descendants' && (
          <PersonPicker label={t('sharing.root')} value={root} onChange={setRoot} error={fields.rootPersonId} />
        )}
        <Choice<SharePrivacy>
          legend={t('sharing.privacy')}
          value={privacy}
          onChange={setPrivacy}
          options={[
            { value: 'deceased', label: t('sharing.privacy_deceased'), hint: t('sharing.privacyDeceasedHint') },
            { value: 'living_names', label: t('sharing.privacy_living_names'), hint: t('sharing.privacyLivingNamesHint') },
          ]}
        />
        <TextField
          type="date"
          label={t('sharing.expiresOn')}
          hint={t('sharing.expiresOnHint')}
          value={expiresOn}
          onChange={(e) => setExpiresOn(e.target.value)}
          error={fields.expiresOn}
        />
      </Form>
    </Dialog>
  )
}
