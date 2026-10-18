import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import {
  addRegion,
  deleteMedia,
  deleteRegion,
  getMedia,
  linkMedia,
  mediaFileUrl,
  setPortrait,
  thumbUrl,
  unlinkMedia,
  updateMedia,
  updateRegion,
} from '../api/endpoints'
import type { Media, MediaLink, MediaRegion, PersonRef } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { TextAreaField } from '../components/Field'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { fullName } from '../lib/people'
import { FaceTagger, type Box } from '../media/FaceTagger'
import { AudioPlayer, VideoPlayer } from '../media/Players'
import { DateField } from '../people/DateField'
import { EventDialog } from '../people/EventDialog'
import { PersonPicker } from '../people/PersonPicker'
import { NotFound } from './NotFound'
import { errorText, formErrors } from '../lib/errors'

export function MediaPage() {
  const { t } = useTranslation()
  const id = Number(useParams().id)
  const media = useQuery({
    queryKey: ['media', 'item', id],
    queryFn: ({ signal }) => getMedia(id, signal),
  })

  if (media.isError) {
    if (media.error instanceof ApiError && media.error.status === 404) return <NotFound />
    return <p role="alert">{t('common.loadFailed')}</p>
  }
  if (media.isPending) return <p role="status">{t('app.loading')}</p>
  return <MediaView media={media.data} />
}

function MediaView({ media: m }: { media: Media }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const title = m.title || m.originalName || t(`media.kinds.${m.kind}`)
  const [drawing, setDrawing] = useState(false)
  const [pending, setPending] = useState<Box | null>(null)
  const [selected, setSelected] = useState<number | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [eventFor, setEventFor] = useState<number | null>(null)

  const refresh = (updated?: Media) => {
    if (updated) queryClient.setQueryData(['media', 'item', m.id], updated)
    void queryClient.invalidateQueries({ queryKey: ['media'] })
    void queryClient.invalidateQueries({ queryKey: ['person'] })
    void queryClient.invalidateQueries({ queryKey: ['persons'] })
    void queryClient.invalidateQueries({ queryKey: ['tree'] })
  }
  const remove = useMutation({
    mutationFn: () => deleteMedia(m.id),
    onSuccess: () => {
      refresh()
      navigate(-1)
    },
  })

  const people = m.links.filter((l) => l.entityType === 'person' && l.personId !== null)

  return (
    <article className="max-w-5xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <PageHeading title={title}>{title}</PageHeading>
        <div className="flex flex-wrap gap-2">
          <a
            href={`${mediaFileUrl(m.id)}?download=1`}
            className="inline-flex min-h-11 items-center rounded-lg border border-slate-400 px-4 font-medium hover:bg-slate-100 dark:border-slate-600 dark:hover:bg-slate-800"
          >
            {t('media.download')}
          </a>
          <Button variant="ghost" onClick={() => setDeleting(true)}>
            {t('media.delete')}
          </Button>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="space-y-3">
          {m.kind === 'image' && (
            <>
              <FaceTagger
                media={m}
                src={thumbUrl(m.id, 1024)}
                alt={m.description || title}
                drawing={drawing}
                pending={pending}
                onDrawn={(box) => {
                  setPending(box)
                  setDrawing(false)
                }}
                selectedRegion={selected}
                onSelectRegion={setSelected}
              />
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant={drawing ? 'primary' : 'secondary'}
                  aria-pressed={drawing}
                  onClick={() => setDrawing(!drawing)}
                >
                  {drawing ? t('media.drawingOn') : t('media.tagFace')}
                </Button>
                <a
                  href={mediaFileUrl(m.id)}
                  className="text-brand-700 underline underline-offset-4 dark:text-brand-100"
                >
                  {t('media.original')}
                </a>
              </div>
              {drawing && <p className="text-sm text-slate-600 dark:text-slate-400">{t('media.drawHint')}</p>}
              {pending && (
                <NewRegion
                  media={m}
                  box={pending}
                  onDone={(u) => {
                    setPending(null)
                    if (u) refresh(u)
                  }}
                />
              )}
            </>
          )}
          {m.kind === 'audio' && <AudioPlayer label={title} src={mediaFileUrl(m.id)} />}
          {m.kind === 'video' && <VideoPlayer label={title} src={mediaFileUrl(m.id)} />}
          {m.kind === 'document' && (
            <p>
              <a href={mediaFileUrl(m.id)} className="text-brand-700 underline underline-offset-4 dark:text-brand-100">
                {t('media.openDocument', { name: m.originalName || title })}
              </a>
            </p>
          )}
          {m.transcript && (
            <Section title={t('media.transcript')}>
              <p className="whitespace-pre-line">{m.transcript}</p>
            </Section>
          )}
        </div>

        <div className="space-y-6">
          <Details media={m} onSaved={refresh} />

          {(m.takenAt || m.lat !== null) && (
            <Section title={t('media.camera')}>
              {m.takenAt && <p>{t('media.takenAt', { date: m.takenAt })}</p>}
              {m.lat !== null && m.lng !== null && (
                <p>
                  <a
                    href={`https://www.openstreetmap.org/?mlat=${m.lat}&mlon=${m.lng}#map=15/${m.lat}/${m.lng}`}
                    className="text-brand-700 underline underline-offset-4 dark:text-brand-100"
                    rel="noreferrer"
                    target="_blank"
                  >
                    {t('media.location', {
                      lat: m.lat.toFixed(5),
                      lng: m.lng.toFixed(5),
                    })}
                  </a>
                </p>
              )}
              {m.takenAt &&
                people.map((l) => (
                  <Button key={l.entityId} variant="secondary" className="mt-2" onClick={() => setEventFor(l.entityId)}>
                    {t('media.addAsEvent', { name: l.label })}
                  </Button>
                ))}
            </Section>
          )}

          {m.kind === 'image' && (
            <Section title={t('media.faces')}>
              {m.regions.length === 0 && (
                <p className="text-sm text-slate-600 dark:text-slate-400">{t('media.noFaces')}</p>
              )}
              <ul className="space-y-3">
                {m.regions.map((r) => (
                  <RegionItem key={r.id} media={m} region={r} selected={selected === r.id} onChanged={refresh} />
                ))}
              </ul>
            </Section>
          )}

          <Links media={m} onChanged={refresh} />
        </div>
      </div>

      <ConfirmDialog
        open={deleting}
        title={t('media.delete')}
        message={t('media.deleteConfirm', { title })}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setDeleting(false)}
      />
      {eventFor !== null && (
        <EventDialog
          open
          onClose={() => setEventFor(null)}
          owner={{ personId: eventFor }}
          defaultType="EVEN"
          defaultDate={m.takenAt.slice(0, 10)}
          defaultDescription={title}
        />
      )}
    </article>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  const id = useId()
  return (
    <section aria-labelledby={id} className="space-y-2">
      <h2 id={id} className="border-b border-slate-200 pb-1 text-lg font-semibold dark:border-slate-800">
        {title}
      </h2>
      {children}
    </section>
  )
}

function Details({ media: m, onSaved }: { media: Media; onSaved: (m: Media) => void }) {
  const { t } = useTranslation()
  const [title, setTitle] = useState(m.title)
  const [date, setDate] = useState(m.date)
  const [description, setDescription] = useState(m.description)
  const [transcript, setTranscript] = useState(m.transcript)
  const save = useMutation({
    mutationFn: () => updateMedia(m.id, { title, date, description, transcript }),
    onSuccess: onSaved,
  })
  const errors = formErrors(save.error, t, (msg) => t('common.saveFailed', { message: msg }))
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <h2 className="border-b border-slate-200 pb-1 text-lg font-semibold dark:border-slate-800">
        {t('media.details')}
      </h2>
      <TextField
        label={t('source.title')}
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        error={errors.fields.title}
      />
      <DateField value={date} onChange={setDate} error={errors.fields.date} />
      <TextAreaField
        label={t('media.description')}
        hint={t('media.descriptionHint')}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
      />
      <TextAreaField
        label={t('media.transcript')}
        hint={m.kind === 'audio' || m.kind === 'video' ? t('media.transcriptHintAudio') : t('media.transcriptHint')}
        value={transcript}
        onChange={(e) => setTranscript(e.target.value)}
        rows={m.kind === 'audio' ? 6 : 3}
      />
      {errors.form && (
        <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
          {errors.form}
        </p>
      )}
      <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
        {save.isSuccess && !save.isPending ? t('media.saved') : ''}
      </p>
      <Button type="submit" busy={save.isPending}>
        {t('common.save')}
      </Button>
    </form>
  )
}

function NewRegion({ media: m, box, onDone }: { media: Media; box: Box; onDone: (m?: Media) => void }) {
  const { t } = useTranslation()
  const [person, setPerson] = useState<PersonRef | null>(null)
  const [name, setName] = useState('')
  const save = useMutation({
    mutationFn: () => addRegion(m.id, { personId: person?.id ?? null, name, ...box }),
    onSuccess: (u) => onDone(u),
  })
  return (
    <div className="space-y-3 rounded-xl border border-amber-400 p-3">
      <p className="font-medium">{t('media.whoIsThis')}</p>
      <PersonPicker label={t('media.person')} value={person} onChange={setPerson} />
      {!person && (
        <TextField
          label={t('media.faceName')}
          hint={t('media.faceNameHint')}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      )}
      {save.error && (
        <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
          {errorText(save.error, t)}
        </p>
      )}
      <div className="flex gap-2">
        <Button onClick={() => save.mutate()} busy={save.isPending}>
          {t('media.saveFace')}
        </Button>
        <Button variant="secondary" onClick={() => onDone()}>
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  )
}

function RegionItem({
  media: m,
  region: r,
  selected,
  onChanged,
}: {
  media: Media
  region: MediaRegion
  selected: boolean
  onChanged: (m?: Media) => void
}) {
  const { t } = useTranslation()
  const [matching, setMatching] = useState<PersonRef | null>(null)
  const match = useMutation({
    mutationFn: (p: PersonRef) =>
      updateRegion(r.id, {
        personId: p.id,
        name: r.name,
        x: r.x,
        y: r.y,
        w: r.w,
        h: r.h,
      }),
    onSuccess: (u) => onChanged(u),
  })
  const remove = useMutation({
    mutationFn: () => deleteRegion(r.id),
    onSuccess: () => onChanged(),
  })
  const portrait = useMutation({
    mutationFn: (personId: number) => setPortrait(personId, m.id, r.id),
    onSuccess: () => onChanged(),
  })

  return (
    <li
      className={[
        'space-y-2 rounded-lg border p-2',
        selected ? 'border-amber-400' : 'border-slate-200 dark:border-slate-800',
      ].join(' ')}
    >
      <div className="flex items-center gap-2">
        <img src={thumbUrl(m.id, 128, r.id)} alt="" className="size-12 rounded object-cover" />
        <div className="min-w-0">
          {r.person ? (
            <Link
              to={`/people/${r.person.id}`}
              className="font-medium text-brand-700 underline underline-offset-4 dark:text-brand-100"
            >
              {fullName(r.person) ?? t('person.unknown')}
            </Link>
          ) : (
            <span className="italic">{r.name || t('media.unnamedFace')}</span>
          )}
          {r.source === 'xmp' && (
            <span className="block text-xs text-slate-600 dark:text-slate-400">{t('media.imported')}</span>
          )}
        </div>
      </div>
      {!r.person && (
        <PersonPicker
          label={t('media.matchFace', {
            name: r.name || t('media.unnamedFace'),
          })}
          value={matching}
          onChange={(p) => {
            setMatching(p)
            if (p) match.mutate(p)
          }}
        />
      )}
      <div className="flex flex-wrap gap-2">
        {r.person && (
          <Button
            variant="secondary"
            busy={portrait.isPending}
            onClick={() => r.person && portrait.mutate(r.person.id)}
          >
            {t('media.usePortrait')}
            <span className="sr-only">: {fullName(r.person)}</span>
          </Button>
        )}
        <Button variant="ghost" busy={remove.isPending} onClick={() => remove.mutate()}>
          {t('media.removeFace')}
        </Button>
      </div>
    </li>
  )
}

function Links({ media: m, onChanged }: { media: Media; onChanged: (m?: Media) => void }) {
  const { t } = useTranslation()
  const [adding, setAdding] = useState<PersonRef | null>(null)
  const link = useMutation({
    mutationFn: (p: PersonRef) => linkMedia(m.id, { entityType: 'person', entityId: p.id }),
    onSuccess: (u) => {
      setAdding(null)
      onChanged(u)
    },
  })
  const unlink = useMutation({
    mutationFn: (l: MediaLink) => unlinkMedia(m.id, l),
    onSuccess: () => onChanged(),
  })
  const portrait = useMutation({
    mutationFn: (personId: number) => setPortrait(personId, m.id, null),
    onSuccess: () => onChanged(),
  })

  return (
    <Section title={t('media.shownWith')}>
      {m.links.length === 0 && <p className="text-sm text-slate-600 dark:text-slate-400">{t('media.notLinked')}</p>}
      <ul className="space-y-2">
        {m.links.map((l) => (
          <li key={`${l.entityType}-${l.entityId}`} className="flex flex-wrap items-center justify-between gap-2">
            <LinkLabel link={l} />
            <span className="flex gap-1">
              {l.entityType === 'person' && m.kind === 'image' && (
                <Button variant="ghost" onClick={() => portrait.mutate(l.entityId)}>
                  {t('media.usePortraitShort')}
                  <span className="sr-only">: {l.label}</span>
                </Button>
              )}
              <Button variant="ghost" onClick={() => unlink.mutate(l)}>
                {t('common.remove')}
                <span className="sr-only">: {l.label}</span>
              </Button>
            </span>
          </li>
        ))}
      </ul>
      <PersonPicker
        label={t('media.linkPerson')}
        value={adding}
        exclude={m.links.filter((l) => l.entityType === 'person').map((l) => l.entityId)}
        onChange={(p) => {
          setAdding(p)
          if (p) link.mutate(p)
        }}
      />
    </Section>
  )
}

function LinkLabel({ link }: { link: MediaLink }) {
  const { t } = useTranslation()
  let text = link.label
  if (link.entityType === 'event') {
    const [type = '', name = ''] = link.label.split('|')
    const [tag = '', custom] = type.split(':')
    text = t('citation.eventOf', {
      event: custom || t(`eventType.${tag}`, { defaultValue: tag }),
      name: name || t('person.unknown'),
    })
  } else if (link.entityType === 'family') {
    text = t('citation.target.family', {
      name: link.label || t('person.unknown'),
    })
  }
  if (link.entityType === 'source' || link.entityType === 'heirloom') {
    return (
      <Link
        to={`/${link.entityType === 'source' ? 'sources' : 'heirlooms'}/${link.entityId}`}
        className="text-brand-700 underline underline-offset-4 dark:text-brand-100"
      >
        {text}
      </Link>
    )
  }
  return link.personId !== null ? (
    <Link to={`/people/${link.personId}`} className="text-brand-700 underline underline-offset-4 dark:text-brand-100">
      {text || t('person.unknown')}
    </Link>
  ) : (
    <span>{text}</span>
  )
}
