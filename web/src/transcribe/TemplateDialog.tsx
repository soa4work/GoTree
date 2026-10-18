import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createTemplate, deleteTemplate, updateTemplate } from '../api/endpoints'
import type { ColumnKind, RecordTemplate, RoleKind, TemplateRole } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { SelectField } from '../components/Field'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { familyEventTypes, participantRoles, personEventTypes } from '../lib/eventTypes'
import { errorText, fieldErrors } from '../lib/errors'

const columnKinds: ColumnKind[] = ['given', 'surname', 'sex', 'age', 'occupation', 'residence', 'birthplace', 'notes']
const roleKinds: RoleKind[] = ['principal', 'partner', 'parent', 'participant']

interface Props {
  template?: RecordTemplate
  onClose: () => void
}

/** Creates or edits a custom record template. */
export function TemplateDialog({ template, onClose }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const id = template ? Number(template.key.replace('custom:', '')) : null
  const [name, setName] = useState(template?.name ?? '')
  const [eventType, setEventType] = useState(template?.eventType ?? 'EVEN')
  const [roles, setRoles] = useState<(TemplateRole & { uid: number })[]>(
    (template?.roles ?? [{ key: 'r1', label: '', kind: 'principal', participant: '' }]).map((r, i) => ({
      ...r,
      uid: i,
    })),
  )
  const [columns, setColumns] = useState<ColumnKind[]>(template?.columns ?? ['given', 'surname'])
  const [confirmDelete, setConfirmDelete] = useState(false)
  const nextUid = () => Math.max(-1, ...roles.map((r) => r.uid)) + 1
  const done = () => {
    void queryClient.invalidateQueries({ queryKey: ['templates'] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => {
      const tpl: RecordTemplate = {
        key: '',
        name,
        builtin: false,
        eventType,
        columns: columnKinds.filter((c) => columns.includes(c)),
        roles: roles.map(({ uid, ...r }) => ({ ...r, key: r.key || `r${uid}` })),
      }
      return id ? updateTemplate(id, tpl) : createTemplate(tpl)
    },
    onSuccess: done,
  })
  const remove = useMutation({ mutationFn: () => deleteTemplate(id ?? 0), onSuccess: done })
  const fields = fieldErrors(save.error, t)
  const setRole = (uid: number, patch: Partial<TemplateRole>) =>
    setRoles((prev) => prev.map((r) => (r.uid === uid ? { ...r, ...patch } : r)))
  const columnsId = useId()

  return (
    <>
      <Dialog open onClose={onClose} title={template ? t('transcribe.editTemplate') : t('transcribe.newTemplate')} wide>
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
          secondary={
            template && (
              <Button type="button" variant="ghost" onClick={() => setConfirmDelete(true)}>
                {t('transcribe.deleteTemplate')}
              </Button>
            )
          }
        >
          <TextField
            label={t('transcribe.templateName')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={fields.name}
          />
          <SelectField
            label={t('transcribe.eventType')}
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            error={fields.eventType}
          >
            {[...new Set<string>([...personEventTypes, ...familyEventTypes])].map((tag) => (
              <option key={tag} value={tag}>
                {t(`eventType.${tag}`, { defaultValue: tag })}
              </option>
            ))}
          </SelectField>
          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">{t('transcribe.roles')}</legend>
            {fields.roles && (
              <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
                {fields.roles}
              </p>
            )}
            {roles.map((r, i) => (
              <div
                key={r.uid}
                className="grid gap-3 rounded-lg border border-slate-200 p-3 sm:grid-cols-3 dark:border-slate-800"
              >
                <TextField
                  label={t('transcribe.roleLabel')}
                  value={r.label}
                  onChange={(e) => setRole(r.uid, { label: e.target.value })}
                />
                <SelectField
                  label={t('transcribe.roleKind')}
                  value={r.kind}
                  onChange={(e) => setRole(r.uid, { kind: e.target.value as RoleKind })}
                >
                  {roleKinds.map((k) => (
                    <option key={k} value={k}>
                      {t(`transcribe.kind.${k}`)}
                    </option>
                  ))}
                </SelectField>
                {(r.kind === 'parent' || r.kind === 'participant') && (
                  <SelectField
                    label={t('transcribe.roleParticipant')}
                    value={r.participant}
                    onChange={(e) => setRole(r.uid, { participant: e.target.value })}
                  >
                    <option value="">—</option>
                    {participantRoles.map((p) => (
                      <option key={p} value={p}>
                        {t(`role.${p}`, { defaultValue: p })}
                      </option>
                    ))}
                  </SelectField>
                )}
                {roles.length > 1 && (
                  <Button
                    type="button"
                    variant="ghost"
                    className="sm:col-span-3 sm:justify-self-start"
                    onClick={() => setRoles((prev) => prev.filter((x) => x.uid !== r.uid))}
                  >
                    {t('transcribe.removeRole', { n: i + 1 })}
                  </Button>
                )}
              </div>
            ))}
            <Button
              type="button"
              variant="secondary"
              onClick={() =>
                setRoles((prev) => [
                  ...prev,
                  { key: '', label: '', kind: 'participant', participant: 'other', uid: nextUid() },
                ])
              }
            >
              + {t('transcribe.addRole')}
            </Button>
          </fieldset>
          <fieldset aria-describedby={fields.columns ? `${columnsId}-error` : undefined}>
            <legend className="text-sm font-medium">{t('transcribe.columns')}</legend>
            {fields.columns && (
              <p id={`${columnsId}-error`} className="text-sm font-medium text-red-700 dark:text-red-400">
                {fields.columns}
              </p>
            )}
            <div className="grid gap-x-4 sm:grid-cols-2">
              {columnKinds.map((c) => (
                <label key={c} className="flex min-h-11 items-center gap-3">
                  <input
                    type="checkbox"
                    className="size-5"
                    checked={columns.includes(c)}
                    onChange={(e) =>
                      setColumns((prev) => (e.target.checked ? [...prev, c] : prev.filter((x) => x !== c)))
                    }
                  />
                  {t(`transcribe.column.${c}`)}
                </label>
              ))}
            </div>
          </fieldset>
        </Form>
      </Dialog>
      <ConfirmDialog
        open={confirmDelete}
        title={t('transcribe.deleteTemplate')}
        message={t('transcribe.deleteTemplateConfirm', { name: template?.name ?? '' })}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </>
  )
}
