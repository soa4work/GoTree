import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { ApiError } from '../api/client'
import {
  applyTranscription,
  createTranscription,
  deleteTranscription,
  getTranscription,
  listTemplates,
  matchRows,
  updateTranscription,
} from '../api/endpoints'
import type {
  ApplyResult,
  MatchCandidate,
  PersonRef,
  PlaceRef,
  RecordTemplate,
  Source,
  Transcription,
  TranscriptionInput,
  TranscriptionRow,
} from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { SelectField, TextAreaField } from '../components/Field'
import { PageHeading } from '../components/PageHeading'
import { TextField } from '../components/TextField'
import { fullName, lifespan } from '../lib/people'
import { DateField } from '../people/DateField'
import { PlaceField } from '../people/PlaceField'
import { SourcePicker } from '../sources/SourcePicker'
import { roleLabel, templateName } from '../transcribe/labels'
import { NotFound } from './NotFound'
import { errorText, fieldErrors } from '../lib/errors'

const emptyRow = (): TranscriptionRow => ({ line: '', role: '', values: {}, action: 'new', personId: null })

/** New transcriptions (/transcribe/new?template=census) and drafts. */
export function TranscriptionPage() {
  const { t } = useTranslation()
  const id = Number(useParams().id) || null
  const templates = useQuery({ queryKey: ['templates'], queryFn: ({ signal }) => listTemplates(signal) })
  const existing = useQuery({
    queryKey: ['transcriptions', id],
    queryFn: ({ signal }) => getTranscription(id ?? 0, signal),
    enabled: id !== null,
  })
  if (existing.error instanceof ApiError && existing.error.status === 404) return <NotFound />
  if (templates.isPending || (id !== null && existing.isPending)) return <p role="status">{t('app.loading')}</p>
  if (templates.isError || existing.isError) return <p role="alert">{t('common.loadFailed')}</p>
  return <Editor key={existing.data?.id ?? 'new'} templates={templates.data} initial={existing.data ?? null} />
}

function Editor({ templates, initial }: { templates: RecordTemplate[]; initial: Transcription | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [params] = useSearchParams()
  const [templateKey, setTemplateKey] = useState(initial?.templateKey ?? params.get('template') ?? 'census')
  const [title, setTitle] = useState(initial?.title ?? '')
  const [source, setSource] = useState<Source | null>(
    initial?.source ? ({ id: initial.source.id, title: initial.source.title } as Source) : null,
  )
  const [page, setPage] = useState(initial?.page ?? '')
  const [date, setDate] = useState(initial?.date ?? '')
  const [place, setPlace] = useState<PlaceRef | null>(initial?.place ?? null)
  const [notes, setNotes] = useState(initial?.notes ?? '')
  const [rows, setRows] = useState<TranscriptionRow[]>(initial?.rows.length ? initial.rows : [emptyRow()])
  // Stable keys for the row cards, which can be removed from the middle.
  const [keys, setKeys] = useState<number[]>(() => rows.map((_, i) => i))
  const [candidates, setCandidates] = useState<MatchCandidate[][] | null>(null)
  const [picked, setPicked] = useState<Record<number, PersonRef>>(initial?.persons ?? {})
  const [applied, setApplied] = useState<ApplyResult | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  // The built-in templates always exist, so the list is never empty.
  const tpl = templates.find((x) => x.key === templateKey) ?? (templates[0] as RecordTemplate)
  const readOnly = initial?.status === 'applied'

  const input = (): TranscriptionInput => ({
    templateKey,
    title,
    sourceId: source?.id ?? null,
    page,
    date,
    placeId: place?.id ?? null,
    notes,
    rows,
  })
  const persist = async () => {
    const saved = initial ? await updateTranscription(initial.id, input()) : await createTranscription(input())
    void queryClient.invalidateQueries({ queryKey: ['transcriptions'] })
    return saved
  }
  const save = useMutation({
    mutationFn: persist,
    onSuccess: (saved) => {
      if (!initial) {
        queryClient.setQueryData(['transcriptions', saved.id], saved)
        navigate(`/transcribe/${saved.id}`, { replace: true })
      }
    },
  })
  const apply = useMutation({
    mutationFn: async () => applyTranscription((await persist()).id),
    onSuccess: (res) => {
      setApplied(res)
      void queryClient.invalidateQueries()
    },
  })
  const find = useMutation({
    mutationFn: () => matchRows(date, rows),
    onSuccess: (found) => {
      setCandidates(found)
      // A strong match is preselected; everything else stays a new person.
      const strong: Record<number, PersonRef> = {}
      setRows(
        rows.map((r, i) => {
          const best = found[i]?.[0]
          if (r.action !== 'new' || !best || best.score < 0.8) return r
          strong[best.person.id] = best.person
          return { ...r, action: 'person', personId: best.person.id }
        }),
      )
      setPicked((p) => ({ ...p, ...strong }))
    },
  })
  const remove = useMutation({
    mutationFn: () => deleteTranscription(initial?.id ?? 0),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['transcriptions'] })
      navigate('/transcribe')
    },
  })
  const error = save.error ?? apply.error
  const fields = fieldErrors(error, t)
  const setRow = (i: number, patch: Partial<TranscriptionRow>) =>
    setRows((prev) => prev.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const heading = readOnly || applied ? t('transcribe.appliedTitle') : t('transcribe.editTitle')
  const recordId = useId()
  const peopleId = useId()

  if (applied) {
    return (
      <div className="max-w-3xl space-y-4">
        <PageHeading title={heading}>{heading}</PageHeading>
        <p role="status" className="font-medium text-green-800 dark:text-green-300">
          {t('transcribe.appliedMessage', {
            facts: t('transcribe.appliedFacts', { count: applied.facts }),
            people: t('transcribe.appliedPeople', { count: applied.created.length }),
          })}
        </p>
        <ul className="space-y-1">
          {applied.rows
            .flatMap((r) => (r.personId === null ? [] : [r.personId]))
            .map((pid) => {
              const p = applied.persons[pid]
              return (
                <li key={pid}>
                  <Link
                    to={`/people/${pid}`}
                    className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100"
                  >
                    {t('transcribe.openPerson', { name: (p && fullName(p)) ?? t('person.unknown') })}
                  </Link>
                </li>
              )
            })}
        </ul>
        <Link to="/transcribe" className="inline-flex min-h-11 items-center underline">
          {t('transcribe.list')}
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-3xl space-y-8">
      <PageHeading title={heading}>{heading}</PageHeading>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
        className="space-y-8"
      >
        <fieldset disabled={readOnly} className="space-y-8">
          <section aria-labelledby={recordId} className="space-y-4">
            <h2 id={recordId} className="text-xl font-semibold">
              {t('transcribe.record')}
            </h2>
            <SelectField
              label={t('transcribe.template')}
              value={templateKey}
              disabled={!!initial}
              onChange={(e) => setTemplateKey(e.target.value)}
              error={fields.templateKey}
            >
              {templates.map((x) => (
                <option key={x.key} value={x.key}>
                  {templateName(t, x)}
                </option>
              ))}
            </SelectField>
            <TextField
              label={t('transcribe.title')}
              hint={t('transcribe.titleHint')}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              error={fields.title}
            />
            {readOnly ? (
              <p>{initial?.source?.title}</p>
            ) : (
              <SourcePicker value={source} onChange={setSource} error={fields.sourceId} />
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField
                label={t('transcribe.page')}
                value={page}
                onChange={(e) => setPage(e.target.value)}
                error={fields.page}
              />
              <DateField value={date} onChange={setDate} error={fields.date} />
            </div>
            {!readOnly && <PlaceField value={place} onChange={setPlace} error={fields.placeId} />}
            <TextAreaField
              label={t('transcribe.notes')}
              hint={t('transcribe.notesHint')}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </section>

          <section aria-labelledby={peopleId} className="space-y-4">
            <h2 id={peopleId} className="text-xl font-semibold">
              {t('transcribe.people')}
            </h2>
            {fields.rows && (
              <p role="alert" className="font-medium text-red-700 dark:text-red-400">
                {fields.rows}
              </p>
            )}
            <ol className="space-y-4">
              {rows.map((r, i) => (
                <li key={keys[i]}>
                  <RowEditor
                    n={i + 1}
                    row={r}
                    tpl={tpl}
                    error={fields[`rows.${i}`]}
                    candidates={candidates?.[i]}
                    picked={picked}
                    readOnly={readOnly}
                    onChange={(patch) => setRow(i, patch)}
                    onPick={(p) => {
                      setPicked((prev) => ({ ...prev, [p.id]: p }))
                      setRow(i, { action: 'person', personId: p.id })
                    }}
                    onRemove={
                      rows.length > 1
                        ? () => {
                            setRows((prev) => prev.filter((_, j) => j !== i))
                            setKeys((prev) => prev.filter((_, j) => j !== i))
                            setCandidates(null)
                          }
                        : undefined
                    }
                  />
                </li>
              ))}
            </ol>
            {!readOnly && (
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="secondary" onClick={() => {
                    setRows((prev) => [...prev, emptyRow()])
                    setKeys((prev) => [...prev, Math.max(-1, ...prev) + 1])
                  }}
                >
                  + {t('transcribe.addRow')}
                </Button>
                <Button type="button" variant="secondary" busy={find.isPending} onClick={() => find.mutate()}>
                  {t('transcribe.findMatches')}
                </Button>
              </div>
            )}
            <p role="status" className="text-sm text-slate-600 dark:text-slate-400">
              {find.isPending ? t('transcribe.matching') : find.isSuccess ? t('transcribe.matchesFound') : ''}
            </p>
            {find.isError && (
              <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
                {t('common.loadFailed')}
              </p>
            )}
          </section>
        </fieldset>

        {!readOnly && (
          <div className="space-y-3 border-t border-slate-200 pt-4 dark:border-slate-800">
            {error && Object.keys(fields).length === 0 && (
              <p role="alert" className="font-medium text-red-700 dark:text-red-400">
                {t('common.saveFailed', { message: errorText(error, t) })}
              </p>
            )}
            <p role="status" className="text-sm font-medium text-green-800 dark:text-green-300">
              {save.isSuccess ? t('transcribe.saved') : ''}
            </p>
            <p className="text-sm text-slate-600 dark:text-slate-400">{t('transcribe.applyHint')}</p>
            <div className="flex flex-wrap gap-2">
              <Button type="button" busy={apply.isPending} onClick={() => apply.mutate()}>
                {t('transcribe.apply')}
              </Button>
              <Button type="submit" variant="secondary" busy={save.isPending}>
                {t('transcribe.save')}
              </Button>
              {initial && (
                <Button type="button" variant="ghost" onClick={() => setConfirmDelete(true)}>
                  {t('transcribe.delete')}
                </Button>
              )}
            </div>
          </div>
        )}
      </form>
      <ConfirmDialog
        open={confirmDelete}
        title={t('transcribe.delete')}
        message={t('transcribe.deleteConfirm')}
        confirmLabel={t('common.delete')}
        busy={remove.isPending}
        error={remove.error ? t('common.deleteFailed', { message: errorText(remove.error, t) }) : null}
        onConfirm={() => remove.mutate()}
        onClose={() => setConfirmDelete(false)}
      />
    </div>
  )
}

interface RowProps {
  n: number
  row: TranscriptionRow
  tpl: RecordTemplate
  error?: string
  candidates?: MatchCandidate[]
  picked: Record<number, PersonRef>
  readOnly: boolean
  onChange: (patch: Partial<TranscriptionRow>) => void
  onPick: (p: PersonRef) => void
  onRemove?: () => void
}

function RowEditor({ n, row, tpl, error, candidates, picked, readOnly, onChange, onPick, onRemove }: RowProps) {
  const { t } = useTranslation()
  const whoName = useId()
  const linked = row.personId !== null ? picked[row.personId] : undefined
  const shown = new Map<number, PersonRef>()
  for (const c of candidates ?? []) shown.set(c.person.id, c.person)
  if (linked) shown.set(linked.id, linked)
  const percent = (id: number) => {
    const sc = candidates?.find((c) => c.person.id === id)?.score
    return sc === undefined ? null : Math.round(sc * 100)
  }

  return (
    <fieldset className="space-y-3 rounded-xl border border-slate-200 p-4 dark:border-slate-800">
      <legend className="px-1 font-semibold">{t('transcribe.person', { n })}</legend>
      {error && (
        <p role="alert" className="font-medium text-red-700 dark:text-red-400">
          {error}
        </p>
      )}
      <div className="grid gap-3 sm:grid-cols-[6rem_1fr]">
        <TextField label={t('transcribe.line')} value={row.line} onChange={(e) => onChange({ line: e.target.value })} />
        <SelectField label={t('transcribe.role')} value={row.role} onChange={(e) => onChange({ role: e.target.value })}>
          <option value="">{t('transcribe.noRole')}</option>
          {tpl.roles.map((r) => (
            <option key={r.key} value={r.key}>
              {roleLabel(t, tpl, r)}
            </option>
          ))}
        </SelectField>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {tpl.columns.map((c) => (
          <TextField
            key={c}
            label={t(`transcribe.column.${c}`)}
            value={row.values[c] ?? ''}
            onChange={(e) => onChange({ values: { ...row.values, [c]: e.target.value } })}
          />
        ))}
      </div>
      {readOnly
        ? linked && (
            <p>
              <Link
                to={`/people/${linked.id}`}
                className="inline-flex min-h-11 items-center font-medium text-brand-700 underline dark:text-brand-100"
              >
                {t('transcribe.linked', { name: fullName(linked) ?? t('person.unknown') })}
              </Link>
            </p>
          )
        : (candidates !== undefined || row.action !== 'new') && (
            <fieldset className="space-y-1">
              <legend className="text-sm font-medium">{t('transcribe.who')}</legend>
              {candidates?.length === 0 && (
                <p className="text-sm text-slate-600 dark:text-slate-400">{t('transcribe.noMatches')}</p>
              )}
              {[...shown.values()].map((p) => (
                <label key={p.id} className="flex min-h-11 items-center gap-3">
                  <input
                    type="radio"
                    name={whoName}
                    className="size-5"
                    checked={row.action === 'person' && row.personId === p.id}
                    onChange={() => onPick(p)}
                  />
                  <span>
                    <span className="font-medium">{fullName(p) ?? t('person.unknown')}</span>
                    {lifespan(p) && (
                      <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">{lifespan(p)}</span>
                    )}
                    {percent(p.id) !== null && (
                      <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">
                        · {t('transcribe.matchScore', { percent: percent(p.id) })}
                      </span>
                    )}
                  </span>
                </label>
              ))}
              <label className="flex min-h-11 items-center gap-3">
                <input
                  type="radio"
                  name={whoName}
                  className="size-5"
                  checked={row.action === 'new'}
                  onChange={() => onChange({ action: 'new', personId: null })}
                />
                {t('transcribe.newPerson')}
              </label>
              <label className="flex min-h-11 items-center gap-3">
                <input
                  type="radio"
                  name={whoName}
                  className="size-5"
                  checked={row.action === 'skip'}
                  onChange={() => onChange({ action: 'skip', personId: null })}
                />
                {t('transcribe.skip')}
              </label>
            </fieldset>
          )}
      {onRemove && !readOnly && (
        <Button type="button" variant="ghost" onClick={onRemove}>
          {t('transcribe.removeRow', { n })}
        </Button>
      )}
    </fieldset>
  )
}
