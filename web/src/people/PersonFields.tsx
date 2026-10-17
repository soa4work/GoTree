import { useTranslation } from 'react-i18next'
import type { AlternateNameInput, NameType, PersonInput, Sex } from '../api/types'
import { Button } from '../components/Button'
import { SelectField, TextAreaField } from '../components/Field'
import { TextField } from '../components/TextField'
import type { LifeDrafts } from '../lib/people'
import { DateField } from './DateField'
import { PlaceField } from './PlaceField'

interface Props {
  value: PersonInput
  onChange: (value: PersonInput) => void
  errors: Record<string, string>
  /** Show notes and alternate names (the full editor) or only the basics. */
  full?: boolean
  /** Optional birth and death entered with a new person (create forms only). */
  life?: { value: LifeDrafts; onChange: (value: LifeDrafts) => void }
}

const nameTypes: NameType[] = ['birth', 'married', 'aka', 'religious', 'immigrant', 'other']

export function PersonFields({ value, onChange, errors, full, life }: Props) {
  const { t } = useTranslation()
  const set = <K extends keyof PersonInput>(key: K, v: PersonInput[K]) => onChange({ ...value, [key]: v })
  const setName = (i: number, patch: Partial<AlternateNameInput>) =>
    set(
      'alternateNames',
      value.alternateNames.map((n, j) => (j === i ? { ...n, ...patch } : n)),
    )

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label={t('person.givenNames')}
          value={value.givenNames}
          onChange={(e) => set('givenNames', e.target.value)}
          error={errors.givenNames}
          autoComplete="off"
        />
        <TextField
          label={t('person.surname')}
          value={value.surname}
          onChange={(e) => set('surname', e.target.value)}
          error={errors.surname}
          autoComplete="off"
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <SelectField label={t('person.sex')} value={value.sex} onChange={(e) => set('sex', e.target.value as Sex)} error={errors.sex}>
          {(['F', 'M', 'X', 'U'] as const).map((s) => (
            <option key={s} value={s}>
              {t(`sex.${s}`)}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('person.livingLabel')}
          hint={t('person.livingHint')}
          value={value.isLiving === null ? 'auto' : value.isLiving ? 'yes' : 'no'}
          onChange={(e) => set('isLiving', e.target.value === 'auto' ? null : e.target.value === 'yes')}
        >
          <option value="auto">{t('person.livingAuto')}</option>
          <option value="yes">{t('person.livingYes')}</option>
          <option value="no">{t('person.livingNo')}</option>
        </SelectField>
      </div>

      {life && (
        <>
          <LifeFieldset
            legend={t('person.birth')}
            draft={life.value.birth}
            onChange={(birth) => life.onChange({ ...life.value, birth })}
            errors={{ date: errors['birth.date'], place: errors['birth.placeId'] }}
          />
          {value.isLiving !== true && (
            <LifeFieldset
              legend={t('person.death')}
              draft={life.value.death}
              onChange={(death) => life.onChange({ ...life.value, death })}
              errors={{ date: errors['death.date'] ?? errors.death, place: errors['death.placeId'] }}
            />
          )}
        </>
      )}

      {full && (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            <TextField label={t('person.nickname')} value={value.nickname} onChange={(e) => set('nickname', e.target.value)} />
            <TextField label={t('person.namePrefix')} hint={t('person.namePrefixHint')} value={value.namePrefix} onChange={(e) => set('namePrefix', e.target.value)} />
            <TextField label={t('person.nameSuffix')} hint={t('person.nameSuffixHint')} value={value.nameSuffix} onChange={(e) => set('nameSuffix', e.target.value)} />
          </div>

          <fieldset className="space-y-3">
            <legend className="text-sm font-semibold">{t('person.alternateNames')}</legend>
            {errors.alternateNames && (
              <p className="text-sm font-medium text-red-700 dark:text-red-400">{errors.alternateNames}</p>
            )}
            {value.alternateNames.map((n, i) => (
              <div key={i} className="grid gap-3 rounded-lg border border-slate-200 p-3 sm:grid-cols-[10rem_1fr_1fr_auto] sm:items-end dark:border-slate-800">
                <SelectField label={t('person.nameType')} value={n.type} onChange={(e) => setName(i, { type: e.target.value as NameType })}>
                  {nameTypes.map((nt) => (
                    <option key={nt} value={nt}>
                      {t(`nameType.${nt}`)}
                    </option>
                  ))}
                </SelectField>
                <TextField label={t('person.givenNames')} value={n.givenNames} onChange={(e) => setName(i, { givenNames: e.target.value })} />
                <TextField label={t('person.surname')} value={n.surname} onChange={(e) => setName(i, { surname: e.target.value })} />
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => set('alternateNames', value.alternateNames.filter((_, j) => j !== i))}
                >
                  {t('common.remove')}
                  <span className="sr-only">: {t(`nameType.${n.type}`)}</span>
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="ghost"
              onClick={() =>
                set('alternateNames', [
                  ...value.alternateNames,
                  { type: 'birth', givenNames: '', surname: '', namePrefix: '', nameSuffix: '', nickname: '', status: 'accepted', statusReason: '' },
                ])
              }
            >
              + {t('person.addName')}
            </Button>
          </fieldset>

          <TextAreaField label={t('person.notes')} value={value.notes} onChange={(e) => set('notes', e.target.value)} rows={4} />
        </>
      )}
    </div>
  )
}

interface LifeFieldsetProps {
  legend: string
  draft: LifeDrafts['birth']
  onChange: (draft: LifeDrafts['birth']) => void
  errors: { date?: string; place?: string }
}

function LifeFieldset({ legend, draft, onChange, errors }: LifeFieldsetProps) {
  return (
    <fieldset className="grid gap-4 sm:grid-cols-2">
      <legend className="text-sm font-semibold">{legend}</legend>
      <DateField value={draft.date} onChange={(date) => onChange({ ...draft, date })} error={errors.date} />
      <PlaceField value={draft.place} onChange={(place) => onChange({ ...draft, place })} error={errors.place} />
    </fieldset>
  )
}
