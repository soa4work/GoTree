import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createEvent, deleteEvent, updateEvent } from '../api/endpoints'
import type { EventInput, FactStatus, LifeEvent, PersonRef, PlaceRef } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { SelectField, TextAreaField } from '../components/Field'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { familyEventTypes, participantRoles, personEventTypes } from '../lib/eventTypes'
import { CitationEditor, type PendingCitation } from '../sources/CitationEditor'
import { fullName } from '../lib/people'
import { DateField } from './DateField'
import { PersonPicker } from './PersonPicker'
import { PlaceField } from './PlaceField'
import { errorText, formErrors } from '../lib/errors'

export type EventOwner = { personId: number } | { familyId: number }

interface Props {
  open: boolean
  onClose: () => void
  owner: EventOwner
  /** Edit this event, or create a new one when absent. */
  event?: LifeEvent
  /** Preselected values for a new event. */
  defaultType?: string
  defaultDate?: string
  defaultDescription?: string
}

export function EventDialog({ open, onClose, owner, event, defaultType, defaultDate, defaultDescription }: Props) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onClose={onClose} title={event ? t('event.edit') : t('event.new')} wide>
      {open && (
        <EventForm
          owner={owner}
          event={event}
          defaultType={defaultType}
          defaultDate={defaultDate}
          defaultDescription={defaultDescription}
          onClose={onClose}
        />
      )}
    </Dialog>
  )
}

interface ParticipantRow {
  person: PersonRef
  role: string
  customRole: string
  notes: string
}

function EventForm({ owner, event, defaultType, defaultDate, defaultDescription, onClose }: Omit<Props, 'open'>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isFamily = 'familyId' in owner
  const types: readonly string[] = isFamily ? familyEventTypes : personEventTypes

  const [type, setType] = useState(event?.type ?? defaultType ?? types[0] ?? 'EVEN')
  const [customLabel, setCustomLabel] = useState(event?.customLabel ?? '')
  const [date, setDate] = useState(event?.date.raw ?? defaultDate ?? '')
  const [place, setPlace] = useState<PlaceRef | null>(event?.place ?? null)
  const [description, setDescription] = useState(event?.description ?? defaultDescription ?? '')
  const [notes, setNotes] = useState(event?.notes ?? '')
  const [status, setStatus] = useState<FactStatus>(event?.status ?? 'accepted')
  const [statusReason, setStatusReason] = useState(event?.statusReason ?? '')
  const [participants, setParticipants] = useState<ParticipantRow[]>(
    event?.participants.map((p) => ({ person: p.person, role: p.role, customRole: p.customRole, notes: p.notes })) ?? [],
  )
  const [newParticipant, setNewParticipant] = useState<PersonRef | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [addCitations, setAddCitations] = useState<PendingCitation[]>([])
  const [removeCitations, setRemoveCitations] = useState<number[]>([])

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['person'] })
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
    void queryClient.invalidateQueries({ queryKey: ['persons'] })
  }
  const save = useMutation({
    mutationFn: () => {
      const input: EventInput = {
        personId: 'personId' in owner ? owner.personId : null,
        familyId: 'familyId' in owner ? owner.familyId : null,
        type,
        customLabel,
        date,
        placeId: place?.id ?? null,
        description,
        notes,
        status,
        statusReason: status === 'accepted' ? '' : statusReason,
        sortOrder: event?.sortOrder ?? 0,
        participants: participants.map((p) => ({ personId: p.person.id, role: p.role, customRole: p.customRole, notes: p.notes })),
        addCitations: addCitations.map(({ sourceId, page, quality, text }) => ({ sourceId, page, quality, text })),
        removeCitations,
      }
      return event ? updateEvent(event.id, input) : createEvent(input)
    },
    onSuccess: () => {
      invalidate()
      onClose()
    },
  })
  const remove = useMutation({
    mutationFn: () => deleteEvent(event!.id),
    onSuccess: () => {
      invalidate()
      setConfirmDelete(false)
      onClose()
    },
  })
  const errors = formErrors(save.error, t, (m) => t('common.saveFailed', { message: m }))
  const typeLabel = (tag: string) => t(`eventType.${tag}`, { defaultValue: tag })
  const excluded = ['personId' in owner ? owner.personId : 0, ...participants.map((p) => p.person.id)]

  return (
    <>
      <Form
        onSubmit={() => save.mutate()}
        onCancel={onClose}
        submitLabel={t('common.save')}
        busy={save.isPending}
        error={errors.form}
        secondary={
          event && (
            <Button type="button" variant="ghost" onClick={() => setConfirmDelete(true)}>
              {t('event.delete')}
            </Button>
          )
        }
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectField label={t('event.type')} value={type} onChange={(e) => setType(e.target.value)} error={errors.fields.type}>
            {!types.includes(type) && <option value={type}>{typeLabel(type)}</option>}
            {types.map((tag) => (
              <option key={tag} value={tag}>
                {typeLabel(tag)}
              </option>
            ))}
          </SelectField>
          {(type === 'EVEN' || type === 'FACT' || type.startsWith('_')) && (
            <TextField
              label={t('event.customLabel')}
              value={customLabel}
              onChange={(e) => setCustomLabel(e.target.value)}
              error={errors.fields.customLabel}
            />
          )}
        </div>
        <DateField value={date} onChange={setDate} error={errors.fields.date} />
        <PlaceField value={place} onChange={setPlace} error={errors.fields.placeId} />
        <TextField
          label={t('event.description')}
          hint={t('event.descriptionHint')}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          error={errors.fields.description}
        />

        <fieldset className="space-y-3">
          <legend className="text-sm font-semibold">{t('event.participants')}</legend>
          <p className="text-sm text-slate-600 dark:text-slate-400">{t('event.participantsHint')}</p>
          {participants.map((p, i) => (
            <div key={p.person.id} className="grid gap-3 rounded-lg border border-slate-200 p-3 sm:grid-cols-[1fr_12rem_auto] sm:items-end dark:border-slate-800">
              <p className="font-medium">{fullName(p.person) ?? t('person.unknown')}</p>
              <SelectField
                label={t('event.role')}
                value={p.role}
                onChange={(e) => setParticipants(participants.map((q, j) => (j === i ? { ...q, role: e.target.value } : q)))}
              >
                {participantRoles.map((r) => (
                  <option key={r} value={r}>
                    {t(`role.${r}`)}
                  </option>
                ))}
              </SelectField>
              <Button type="button" variant="ghost" onClick={() => setParticipants(participants.filter((_, j) => j !== i))}>
                {t('common.remove')}
                <span className="sr-only">: {fullName(p.person)}</span>
              </Button>
            </div>
          ))}
          {errors.fields.participants && (
            <p className="text-sm font-medium text-red-700 dark:text-red-400">{errors.fields.participants}</p>
          )}
          <PersonPicker
            label={t('event.addParticipant')}
            value={newParticipant}
            exclude={excluded}
            onChange={(p) => {
              if (p) setParticipants([...participants, { person: p, role: 'witness', customRole: '', notes: '' }])
              setNewParticipant(null)
            }}
          />
        </fieldset>

        <CitationEditor
          existing={event?.citations ?? []}
          removed={removeCitations}
          onRemovedChange={setRemoveCitations}
          added={addCitations}
          onAddedChange={setAddCitations}
          error={errors.fields['citations.sourceId'] ?? errors.fields.removeCitations}
        />

        <div className="grid gap-4 sm:grid-cols-2">
          <SelectField
            label={t('event.status')}
            hint={t('event.statusHint')}
            value={status}
            onChange={(e) => setStatus(e.target.value as FactStatus)}
          >
            {(['accepted', 'disputed', 'disproven'] as const).map((s) => (
              <option key={s} value={s}>
                {t(`status.${s}`)}
              </option>
            ))}
          </SelectField>
          {status !== 'accepted' && (
            <TextField
              label={t('event.statusReason')}
              value={statusReason}
              onChange={(e) => setStatusReason(e.target.value)}
              error={errors.fields.statusReason}
            />
          )}
        </div>
        <TextAreaField label={t('person.notes')} value={notes} onChange={(e) => setNotes(e.target.value)} />
      </Form>
      <ConfirmDialog
        open={confirmDelete}
        title={t('event.delete')}
        message={t('event.deleteConfirm', { type: typeLabel(type) })}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </>
  )
}
