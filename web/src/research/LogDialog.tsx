import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { createLogEntry, deleteLogEntry, listTasks, updateLogEntry } from '../api/endpoints'
import type { LogEntry, ResearchLink, SearchResult } from '../api/types'
import { Button } from '../components/Button'
import { Choice } from '../components/Choice'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Dialog } from '../components/Dialog'
import { SelectField, TextAreaField } from '../components/Field'
import { Form } from '../components/Form'
import { TextField } from '../components/TextField'
import { LinkEditor } from './LinkEditor'

interface Props {
  entry?: LogEntry
  initialLinks?: ResearchLink[]
  initialTaskId?: number | null
  onClose: () => void
}

const today = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function LogDialog({ entry, initialLinks = [], initialTaskId = null, onClose }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [searchedOn, setSearchedOn] = useState(entry?.searchedOn ?? today())
  const [query, setQuery] = useState(entry?.query ?? '')
  const [location, setLocation] = useState(entry?.location ?? '')
  const [result, setResult] = useState<SearchResult>(entry?.result ?? 'not_found')
  const [notes, setNotes] = useState(entry?.notes ?? '')
  const [taskId, setTaskId] = useState<number | null>(entry ? entry.taskId : initialTaskId)
  const [links, setLinks] = useState<ResearchLink[]>(entry?.links ?? initialLinks)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const tasks = useQuery({
    queryKey: ['research', 'tasks', 'active'],
    queryFn: ({ signal }) => listTasks({ status: 'active' }, signal),
  })
  const done = () => {
    void queryClient.invalidateQueries({ queryKey: ['research'] })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => {
      const input = {
        taskId,
        searchedOn,
        query,
        location,
        result,
        notes,
        links: links.map(({ entityType, entityId }) => ({ entityType, entityId })),
      }
      return entry ? updateLogEntry(entry.id, input) : createLogEntry(input)
    },
    onSuccess: done,
  })
  const remove = useMutation({ mutationFn: () => deleteLogEntry(entry!.id), onSuccess: done })
  const fields = save.error instanceof ApiError ? save.error.fields : {}
  // The entry's own task may be done by now and missing from the active list.
  const taskOptions = [...(tasks.data ?? [])]
  if (entry?.taskId && !taskOptions.some((x) => x.id === entry.taskId)) {
    taskOptions.push({ id: entry.taskId, title: entry.taskTitle } as (typeof taskOptions)[number])
  }

  // Before the tasks load the select would be empty and then jump to taskId,
  // which a form would take for an edit; hold the value with a placeholder.
  if (taskId && !taskOptions.some((x) => x.id === taskId)) {
    taskOptions.push({ id: taskId, title: t('common.searching') } as (typeof taskOptions)[number])
  }

  return (
    <>
      <Dialog open onClose={onClose} title={entry ? t('research.editEntry') : t('research.newEntry')}>
        <Form
          onSubmit={() => save.mutate()}
          onCancel={onClose}
          submitLabel={t('common.save')}
          busy={save.isPending}
          error={
            save.error && Object.keys(fields).length === 0
              ? t('common.saveFailed', { message: save.error.message })
              : null
          }
          secondary={
            entry && (
              <Button type="button" variant="ghost" onClick={() => setConfirmDelete(true)}>
                {t('research.deleteEntry')}
              </Button>
            )
          }
        >
          <TextField
            label={t('research.query')}
            hint={t('research.queryHint')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            error={fields.query}
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField
              label={t('research.location')}
              hint={t('research.locationHint')}
              value={location}
              onChange={(e) => setLocation(e.target.value)}
              error={fields.location}
            />
            <TextField
              type="date"
              label={t('research.searchedOn')}
              value={searchedOn}
              onChange={(e) => setSearchedOn(e.target.value)}
              error={fields.searchedOn}
            />
          </div>
          <Choice<SearchResult>
            legend={t('research.result')}
            value={result}
            onChange={setResult}
            options={(['found', 'not_found', 'partial'] as const).map((r) => ({
              value: r,
              label: t(`research.result_${r}`),
              hint: t(`research.result_${r}Hint`),
            }))}
          />
          <SelectField
            label={t('research.task')}
            value={taskId ?? ''}
            onChange={(e) => setTaskId(e.target.value ? Number(e.target.value) : null)}
            error={fields.taskId}
          >
            <option value="">{t('research.noTask')}</option>
            {taskOptions.map((task) => (
              <option key={task.id} value={task.id}>
                {task.title}
              </option>
            ))}
          </SelectField>
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
        title={t('research.deleteEntry')}
        message={t('research.deleteEntryConfirm')}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: remove.error.message }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </>
  )
}
