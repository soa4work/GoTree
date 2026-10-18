import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createTask, deleteTask, updateTask } from '../api/endpoints'
import type { ResearchLink, ResearchTask, TaskPriority, TaskStatus } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { SelectField, TextAreaField } from '../components/Field'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { LinkEditor } from './LinkEditor'
import { errorText, fieldErrors } from '../lib/errors'

interface Props {
  task?: ResearchTask
  /** Links for a new task, e.g. the person whose page it was opened from. */
  initialLinks?: ResearchLink[]
  onClose: () => void
}

export function TaskDialog({ task, initialLinks = [], onClose }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [title, setTitle] = useState(task?.title ?? '')
  const [status, setStatus] = useState<TaskStatus>(task?.status ?? 'open')
  const [priority, setPriority] = useState<TaskPriority>(task?.priority ?? 'normal')
  const [dueOn, setDueOn] = useState(task?.dueOn ?? '')
  const [notes, setNotes] = useState(task?.notes ?? '')
  const [links, setLinks] = useState<ResearchLink[]>(task?.links ?? initialLinks)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const done = () => {
    void queryClient.invalidateQueries({ queryKey: ['research'] })
    void queryClient.invalidateQueries({ queryKey: ['person'] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => {
      const input = {
        title,
        status,
        priority,
        dueOn,
        notes,
        origin: task?.origin ?? '',
        links: links.map(({ entityType, entityId }) => ({ entityType, entityId })),
      }
      return task ? updateTask(task.id, input) : createTask(input)
    },
    onSuccess: done,
  })
  const remove = useMutation({ mutationFn: () => deleteTask(task!.id), onSuccess: done })
  const fields = fieldErrors(save.error, t)

  return (
    <>
      <Dialog open onClose={onClose} title={task ? t('research.editTask') : t('research.newTask')}>
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
          task && (
            <Button type="button" variant="ghost" onClick={() => setConfirmDelete(true)}>
              {t('research.deleteTask')}
            </Button>
          )
        }
      >
        <TextField
          label={t('research.title')}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          error={fields.title}
        />
        <div className="grid gap-4 sm:grid-cols-3">
          <SelectField
            label={t('research.status')}
            value={status}
            onChange={(e) => setStatus(e.target.value as TaskStatus)}
          >
            {(['open', 'in_progress', 'done'] as const).map((s) => (
              <option key={s} value={s}>
                {t(`research.status_${s}`)}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t('research.priority')}
            value={priority}
            onChange={(e) => setPriority(e.target.value as TaskPriority)}
          >
            {(['high', 'normal', 'low'] as const).map((p) => (
              <option key={p} value={p}>
                {t(`research.priority_${p}`)}
              </option>
            ))}
          </SelectField>
          <TextField
            type="date"
            label={t('research.dueOn')}
            value={dueOn}
            onChange={(e) => setDueOn(e.target.value)}
            error={fields.dueOn}
          />
        </div>
        <LinkEditor value={links} onChange={setLinks} />
        <TextAreaField
          label={t('research.notes')}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          error={fields.notes}
        />
      </Form>
      </Dialog>
      <ConfirmDialog
        open={confirmDelete}
        title={t('research.deleteTask')}
        message={t('research.deleteTaskConfirm', { title: task?.title ?? '' })}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </>
  )
}
