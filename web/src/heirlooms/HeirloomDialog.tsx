import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createHeirloom, updateHeirloom } from '../api/endpoints'
import type { CustodyHow, Heirloom, HeirloomKind, PersonRef, PlaceRef } from '../api/types'
import { Button } from '../components/Button'
import { Dialog } from '../components/Dialog'
import { SelectField, TextAreaField } from '../components/Field'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { PersonPicker } from '../people/PersonPicker'
import { PlaceField } from '../people/PlaceField'
import { CitationEditor, type PendingCitation } from '../sources/CitationEditor'
import { errorText, fieldErrors } from '../lib/errors'

const kinds: HeirloomKind[] = ['jewellery', 'furniture', 'document', 'photo_album', 'tool', 'textile', 'other']
const ways: CustodyHow[] = ['inherited', 'gift', 'purchased', 'made', 'found', 'other']

interface Holder {
  uid: number
  person: PersonRef | null
  fromDate: string
  toDate: string
  how: CustodyHow
  notes: string
}

interface Props {
  heirloom?: Heirloom
  /** A first holder for a new heirloom, e.g. from a person's page. */
  holder?: PersonRef
  onClose: () => void
  onSaved?: (h: Heirloom) => void
}

export function HeirloomDialog({ heirloom, holder, onClose, onSaved }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState(heirloom?.name ?? '')
  const [kind, setKind] = useState<HeirloomKind>(heirloom?.kind ?? 'other')
  const [description, setDescription] = useState(heirloom?.description ?? '')
  const [madeDate, setMadeDate] = useState(heirloom?.madeDate ?? '')
  const [origin, setOrigin] = useState<PlaceRef | null>(heirloom?.originPlace ?? null)
  const [currentLocation, setCurrentLocation] = useState(heirloom?.currentLocation ?? '')
  const [notes, setNotes] = useState(heirloom?.notes ?? '')
  const [holders, setHolders] = useState<Holder[]>(
    heirloom
      ? heirloom.custody.map((c, i) => ({
          uid: i,
          person: c.person,
          fromDate: c.fromDate,
          toDate: c.toDate,
          how: c.how,
          notes: c.notes,
        }))
      : holder
        ? [{ uid: 0, person: holder, fromDate: '', toDate: '', how: 'inherited', notes: '' }]
        : [],
  )
  const [addCitations, setAddCitations] = useState<PendingCitation[]>([])
  const [removeCitations, setRemoveCitations] = useState<number[]>([])
  const save = useMutation({
    mutationFn: () => {
      const input = {
        name,
        kind,
        description,
        madeDate,
        originPlaceId: origin?.id ?? null,
        currentLocation,
        notes,
        custody: holders.map((h) => ({
          personId: h.person?.id ?? null,
          fromDate: h.fromDate,
          toDate: h.toDate,
          how: h.how,
          notes: h.notes,
        })),
        addCitations: addCitations.map(({ sourceId, page, quality, text }) => ({ sourceId, page, quality, text })),
        removeCitations,
      }
      return heirloom ? updateHeirloom(heirloom.id, input) : createHeirloom(input)
    },
    onSuccess: (h) => {
      void queryClient.invalidateQueries({ queryKey: ['heirlooms'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      onSaved?.(h)
      onClose()
    },
  })
  const fields = fieldErrors(save.error, t)
  const set = (uid: number, patch: Partial<Holder>) =>
    setHolders((prev) => prev.map((h) => (h.uid === uid ? { ...h, ...patch } : h)))
  const move = (i: number) =>
    setHolders((prev) => {
      const next = [...prev]
      const [h] = next.splice(i, 1)
      if (h) next.splice(i - 1, 0, h)
      return next
    })

  return (
    <Dialog open onClose={onClose} title={heirloom ? t('heirloom.edit') : t('heirloom.new')} wide>
      <Form
        onSubmit={() => save.mutate()}
        onCancel={onClose}
        submitLabel={t('common.save')}
        busy={save.isPending}
        error={
          save.error && Object.keys(fields).length === 0
            ? t('common.saveFailed', { message: errorText(save.error, t) })
            : null
        }
      >
        <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
          <TextField
            label={t('heirloom.name')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={fields.name}
          />
          <SelectField
            label={t('heirloom.kind')}
            value={kind}
            onChange={(e) => setKind(e.target.value as HeirloomKind)}
            error={fields.kind}
          >
            {kinds.map((k) => (
              <option key={k} value={k}>
                {t(`heirloom.kinds.${k}`)}
              </option>
            ))}
          </SelectField>
        </div>
        <TextAreaField
          label={t('heirloom.description')}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          error={fields.description}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            label={t('heirloom.madeDate')}
            hint={t('date.hint')}
            value={madeDate}
            onChange={(e) => setMadeDate(e.target.value)}
            error={fields.madeDate}
          />
          <TextField
            label={t('heirloom.currentLocation')}
            value={currentLocation}
            onChange={(e) => setCurrentLocation(e.target.value)}
            error={fields.currentLocation}
          />
        </div>
        <PlaceField value={origin} onChange={setOrigin} error={fields.originPlaceId} />

        <fieldset className="space-y-3">
          <legend className="font-semibold">{t('heirloom.custody')}</legend>
          {fields.custody && (
            <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
              {fields.custody}
            </p>
          )}
          <ol className="space-y-3">
            {holders.map((h, i) => (
              <li key={h.uid}>
                <fieldset className="space-y-3 rounded-lg border border-slate-200 p-3 dark:border-slate-800">
                  <legend className="px-1 text-sm font-medium">{t('heirloom.holder', { n: i + 1 })}</legend>
                  <PersonPicker
                      label={t('heirloom.person')}
                      value={h.person}
                      onChange={(p) => set(h.uid, { person: p })}
                    />
                  {!h.person && (
                    <p className="text-sm text-slate-600 dark:text-slate-400">{t('heirloom.notInTreeHint')}</p>
                  )}
                  <div className="grid gap-3 sm:grid-cols-3">
                    <TextField
                      label={t('heirloom.from')}
                      value={h.fromDate}
                      onChange={(e) => set(h.uid, { fromDate: e.target.value })}
                    />
                    <TextField
                      label={t('heirloom.to')}
                      value={h.toDate}
                      onChange={(e) => set(h.uid, { toDate: e.target.value })}
                    />
                    <SelectField
                      label={t('heirloom.how')}
                      value={h.how}
                      onChange={(e) => set(h.uid, { how: e.target.value as CustodyHow })}
                    >
                      {ways.map((w) => (
                        <option key={w} value={w}>
                          {t(`heirloom.ways.${w}`)}
                        </option>
                      ))}
                    </SelectField>
                  </div>
                  <TextField
                    label={t('heirloom.notes')}
                    value={h.notes}
                    onChange={(e) => set(h.uid, { notes: e.target.value })}
                  />
                  <div className="flex flex-wrap gap-2">
                    {i > 0 && (
                      <Button type="button" variant="ghost" onClick={() => move(i)}>
                        ↑ {t('heirloom.moveUp', { n: i + 1 })}
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setHolders((prev) => prev.filter((x) => x.uid !== h.uid))}
                    >
                      {t('heirloom.removeHolder', { n: i + 1 })}
                    </Button>
                  </div>
                </fieldset>
              </li>
            ))}
          </ol>
          <Button
            type="button"
            variant="secondary"
            onClick={() =>
              setHolders((prev) => [
                ...prev,
                {
                  uid: Math.max(-1, ...prev.map((x) => x.uid)) + 1,
                  person: null,
                  fromDate: '',
                  toDate: '',
                  how: 'inherited',
                  notes: '',
                },
              ])
            }
          >
            + {t('heirloom.addHolder')}
          </Button>
        </fieldset>

        <CitationEditor
          existing={heirloom?.citations ?? []}
          removed={removeCitations}
          onRemovedChange={setRemoveCitations}
          added={addCitations}
          onAddedChange={setAddCitations}
          error={fields['citations.sourceId'] ?? fields.removeCitations}
        />
        <TextAreaField
          label={t('heirloom.notes')}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          error={fields.notes}
        />
      </Form>
    </Dialog>
  )
}
