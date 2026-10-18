import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { deleteFamily, updateFamily } from '../api/endpoints'
import type { Family, UnionType } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { SelectField, TextAreaField } from '../components/Field'
import { Form } from '../components/Form'
import { errorText, formErrors } from '../lib/errors'

interface Props {
  open: boolean
  onClose: () => void
  family: Family
}

export function FamilyDialog({ open, onClose, family }: Props) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onClose={onClose} title={t('family.edit')}>
      {open && <FamilyForm family={family} onClose={onClose} />}
    </Dialog>
  )
}

function FamilyForm({ family, onClose }: Omit<Props, 'open'>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [unionType, setUnionType] = useState<UnionType>(family.unionType)
  const [notes, setNotes] = useState(family.notes)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const done = () => {
    void queryClient.invalidateQueries({ queryKey: ['person'] })
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () =>
      updateFamily(family.id, {
        partner1Id: family.partner1?.id ?? null,
        partner2Id: family.partner2?.id ?? null,
        unionType,
        notes,
      }),
    onSuccess: done,
  })
  const remove = useMutation({ mutationFn: () => deleteFamily(family.id), onSuccess: done })
  const errors = formErrors(save.error, t, (m) => t('common.saveFailed', { message: m }))

  return (
    <>
      <Form
        onSubmit={() => save.mutate()}
        onCancel={onClose}
        submitLabel={t('common.save')}
        busy={save.isPending}
        error={errors.form}
        secondary={
          <Button type="button" variant="ghost" onClick={() => setConfirmDelete(true)}>
            {t('family.delete')}
          </Button>
        }
      >
        <SelectField label={t('family.unionType')} value={unionType} onChange={(e) => setUnionType(e.target.value as UnionType)}>
          {(['married', 'partners', 'unknown'] as const).map((u) => (
            <option key={u} value={u}>
              {t(`unionType.${u}`)}
            </option>
          ))}
        </SelectField>
        <TextAreaField label={t('person.notes')} value={notes} onChange={(e) => setNotes(e.target.value)} />
      </Form>
      <ConfirmDialog
        open={confirmDelete}
        title={t('family.delete')}
        message={t('family.deleteConfirm')}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </>
  )
}
