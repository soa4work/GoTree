import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { addRelative } from '../api/endpoints'
import type { ChildRelation, PersonDetail, PersonInput, PersonRef, Relation, RelativeResult, Source, UnionType } from '../api/types'
import { Dialog } from '../components/Dialog'
import { SelectField } from '../components/Field'
import { Form } from '../components/Form'
import { formErrors } from '../lib/errors'
import { SourcePicker } from '../sources/SourcePicker'
import { TextField } from '../components/TextField'
import { emptyLife, emptyPerson, fullName, lifeInput } from '../lib/people'
import { parentsLabel, partnerLabel } from './familyLabel'
import { PersonFields } from './PersonFields'
import { PersonPicker } from './PersonPicker'

interface Props {
  open: boolean
  onClose: () => void
  anchor: PersonDetail
  relation: Relation
  onSaved: (r: RelativeResult) => void
}

export function RelativeDialog({ open, onClose, anchor, relation, onSaved }: Props) {
  const { t } = useTranslation()
  const name = fullName(anchor) ?? t('person.unknown')
  return (
    <Dialog open={open} onClose={onClose} title={t(`relative.title.${relation}`, { name })} wide>
      {open && <RelativeForm anchor={anchor} relation={relation} onClose={onClose} onSaved={onSaved} />}
    </Dialog>
  )
}

const childRelations: ChildRelation[] = ['birth', 'adopted', 'foster', 'step', 'surrogate', 'unknown']

function RelativeForm({ anchor, relation, onClose, onSaved }: Omit<Props, 'open'>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const modeName = useId()

  const families = relation === 'child' ? anchor.partnerFamilies : relation === 'partner' ? [] : anchor.parentFamilies
  const [mode, setMode] = useState<'new' | 'existing'>('new')
  const [person, setPerson] = useState<PersonInput>(() => ({
    ...emptyPerson(),
    // Children and siblings usually share the surname; a guess that saves typing.
    surname: relation === 'child' || relation === 'sibling' ? anchor.surname : '',
  }))
  const [life, setLife] = useState(emptyLife)
  const [existing, setExisting] = useState<PersonRef | null>(null)
  const [familyId, setFamilyId] = useState<number | null>(families[0]?.id ?? null)
  const [unionType, setUnionType] = useState<UnionType>('married')
  const [childRelation, setChildRelation] = useState<ChildRelation>('birth')
  const [source, setSource] = useState<Source | null>(null)
  const [page, setPage] = useState('')

  const save = useMutation({
    mutationFn: () =>
      addRelative(anchor.id, {
        relation,
        ...(mode === 'new' ? { person: { ...person, ...lifeInput(life, person.isLiving) } } : existing ? { personId: existing.id } : {}),
        ...(familyId !== null && families.length > 0 ? { familyId } : {}),
        ...(relation === 'partner' ? { unionType } : {}),
        ...(relation === 'child' || relation === 'parent' ? { childRelation } : {}),
        ...(source ? { citation: { sourceId: source.id, page, quality: null, text: '' } } : {}),
      }),
    onSuccess: (r) => {
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      void queryClient.invalidateQueries({ queryKey: ['persons'] })
      onSaved(r)
    },
  })
  const errors = formErrors(save.error, (m) => t('common.saveFailed', { message: m }))
  const personError = errors.fields.person ?? errors.fields.personId

  return (
    <Form onSubmit={() => save.mutate()} onCancel={onClose} submitLabel={t('relative.add')} busy={save.isPending} error={errors.form}>
      <fieldset>
        <legend className="text-sm font-semibold">{t('relative.who')}</legend>
        <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:gap-6">
          {(['new', 'existing'] as const).map((m) => (
            <label key={m} className="flex min-h-11 items-center gap-2">
              <input type="radio" name={modeName} value={m} checked={mode === m} onChange={() => setMode(m)} className="size-5" />
              {t(`relative.mode.${m}`)}
            </label>
          ))}
        </div>
      </fieldset>

      {mode === 'new' ? (
        <PersonFields value={person} onChange={setPerson} errors={errors.fields} life={{ value: life, onChange: setLife }} />
      ) : (
        <PersonPicker label={t('relative.pick')} value={existing} onChange={setExisting} exclude={[anchor.id]} error={personError} />
      )}

      {families.length > 1 && (
        <SelectField
          label={relation === 'child' ? t('relative.whichPartnerFamily') : t('relative.whichParentFamily')}
          value={familyId ?? ''}
          onChange={(e) => setFamilyId(Number(e.target.value))}
          error={errors.fields.familyId}
        >
          {families.map((f) => (
            <option key={f.id} value={f.id}>
              {relation === 'child' ? partnerLabel(t, f, anchor.id) : parentsLabel(t, f)}
            </option>
          ))}
        </SelectField>
      )}
      {families.length <= 1 && errors.fields.familyId && (
        <p className="text-sm font-medium text-red-700 dark:text-red-400">{errors.fields.familyId}</p>
      )}

      {relation === 'partner' && (
        <SelectField label={t('family.unionType')} value={unionType} onChange={(e) => setUnionType(e.target.value as UnionType)}>
          {(['married', 'partners', 'unknown'] as const).map((u) => (
            <option key={u} value={u}>
              {t(`unionType.${u}`)}
            </option>
          ))}
        </SelectField>
      )}
      {(relation === 'child' || relation === 'parent') && (
        <SelectField
          label={t('relative.childRelation')}
          value={childRelation}
          onChange={(e) => setChildRelation(e.target.value as ChildRelation)}
          error={errors.fields.childRelation}
        >
          {childRelations.map((r) => (
            <option key={r} value={r}>
              {t(`childRelation.${r}`)}
            </option>
          ))}
        </SelectField>
      )}
      <fieldset className="space-y-3">
        <legend className="text-sm font-semibold">{t('relative.sourceLegend')}</legend>
        <SourcePicker value={source} onChange={setSource} error={errors.fields['citations.sourceId']} />
        {source && <TextField label={t('citation.page')} hint={t('citation.pageHint')} value={page} onChange={(e) => setPage(e.target.value)} />}
      </fieldset>
      {errors.fields.relation && <p className="text-sm font-medium text-red-700 dark:text-red-400">{errors.fields.relation}</p>}
      {errors.fields.childId && <p className="text-sm font-medium text-red-700 dark:text-red-400">{errors.fields.childId}</p>}
    </Form>
  )
}
