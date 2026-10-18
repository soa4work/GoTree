import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createSource, updateSource } from '../api/endpoints'
import type { Repository, Source } from '../api/types'
import { Dialog } from '../components/Dialog'
import { TextAreaField } from '../components/Field'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { RepositoryField } from './RepositoryField'
import { formErrors } from '../lib/errors'

interface Props {
  open: boolean
  onClose: () => void
  source?: Source
  onSaved: (s: Source) => void
}

export function SourceDialog({ open, onClose, source, onSaved }: Props) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onClose={onClose} title={source ? t('source.edit') : t('source.new')} wide>
      {open && <SourceForm source={source} onClose={onClose} onSaved={onSaved} />}
    </Dialog>
  )
}

function SourceForm({ source, onClose, onSaved }: Omit<Props, 'open'>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [title, setTitle] = useState(source?.title ?? '')
  const [author, setAuthor] = useState(source?.author ?? '')
  const [publication, setPublication] = useState(source?.publication ?? '')
  const [callNumber, setCallNumber] = useState(source?.callNumber ?? '')
  const [repository, setRepository] = useState<Repository | null>(source?.repository ?? null)
  const [notes, setNotes] = useState(source?.notes ?? '')

  const save = useMutation({
    mutationFn: () => {
      const input = { title, author, publication, callNumber, repositoryId: repository?.id ?? null, notes }
      return source ? updateSource(source.id, input) : createSource(input)
    },
    onSuccess: (s) => {
      void queryClient.invalidateQueries({ queryKey: ['sources'] })
      void queryClient.invalidateQueries({ queryKey: ['source'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      onSaved(s)
    },
  })
  const errors = formErrors(save.error, t, (m) => t('common.saveFailed', { message: m }))

  return (
    <Form onSubmit={() => save.mutate()} onCancel={onClose} submitLabel={t('common.save')} busy={save.isPending} error={errors.form}>
      <TextField label={t('source.title')} hint={t('source.titleHint')} value={title} onChange={(e) => setTitle(e.target.value)} error={errors.fields.title} required />
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField label={t('source.author')} value={author} onChange={(e) => setAuthor(e.target.value)} error={errors.fields.author} />
        <TextField label={t('source.publication')} hint={t('source.publicationHint')} value={publication} onChange={(e) => setPublication(e.target.value)} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <RepositoryField value={repository} onChange={setRepository} />
        <TextField label={t('source.callNumber')} hint={t('source.callNumberHint')} value={callNumber} onChange={(e) => setCallNumber(e.target.value)} />
      </div>
      <TextAreaField label={t('person.notes')} value={notes} onChange={(e) => setNotes(e.target.value)} />
    </Form>
  )
}
