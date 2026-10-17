import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createPerson, updatePerson } from '../api/endpoints'
import type { Person, PersonInput } from '../api/types'
import { Dialog } from '../components/Dialog'
import { Form } from '../components/Form'
import { formErrors } from '../lib/errors'
import { emptyLife, emptyPerson, lifeInput } from '../lib/people'
import { CitationEditor, type PendingCitation } from '../sources/CitationEditor'
import { PersonFields } from './PersonFields'

interface Props {
  open: boolean
  onClose: () => void
  /** Edit this person, or create a new one when absent. */
  person?: Person
  onSaved: (p: Person) => void
}

const toInput = (p: Person): PersonInput => ({
  givenNames: p.givenNames,
  surname: p.surname,
  namePrefix: p.namePrefix,
  nameSuffix: p.nameSuffix,
  nickname: p.nickname,
  sex: p.sex,
  isLiving: p.isLiving,
  notes: p.notes,
  alternateNames: p.alternateNames.map((n) => ({
    id: n.id,
    type: n.type,
    givenNames: n.givenNames,
    surname: n.surname,
    namePrefix: n.namePrefix,
    nameSuffix: n.nameSuffix,
    nickname: n.nickname,
    status: n.status,
    statusReason: n.statusReason,
  })),
})

export function PersonDialog({ open, onClose, person, onSaved }: Props) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onClose={onClose} title={person ? t('person.edit') : t('person.new')} wide>
      {/* Remounted per opening so the form starts from the current data. */}
      {open && <PersonDialogForm person={person} onClose={onClose} onSaved={onSaved} />}
    </Dialog>
  )
}

function PersonDialogForm({ person, onClose, onSaved }: Omit<Props, 'open'>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [value, setValue] = useState<PersonInput>(person ? toInput(person) : emptyPerson())
  const [life, setLife] = useState(emptyLife)
  const [addCitations, setAddCitations] = useState<PendingCitation[]>([])
  const [removeCitations, setRemoveCitations] = useState<number[]>([])
  const save = useMutation({
    mutationFn: () => {
      const input: PersonInput = {
        ...value,
        ...(person ? {} : lifeInput(life, value.isLiving)),
        addCitations: addCitations.map(({ sourceId, page, quality, text }) => ({ sourceId, page, quality, text })),
        removeCitations,
      }
      return person ? updatePerson(person.id, input) : createPerson(input)
    },
    onSuccess: (p) => {
      void queryClient.invalidateQueries({ queryKey: ['persons'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      onSaved(p)
    },
  })
  const errors = formErrors(save.error, (m) => t('common.saveFailed', { message: m }))

  return (
    <Form onSubmit={() => save.mutate()} onCancel={onClose} submitLabel={t('common.save')} busy={save.isPending} error={errors.form}>
      <PersonFields value={value} onChange={setValue} errors={errors.fields} full life={person ? undefined : { value: life, onChange: setLife }} />
      <CitationEditor
        existing={person?.citations ?? []}
        removed={removeCitations}
        onRemovedChange={setRemoveCitations}
        added={addCitations}
        onAddedChange={setAddCitations}
        error={errors.fields['citations.sourceId'] ?? errors.fields.removeCitations}
      />
    </Form>
  )
}
