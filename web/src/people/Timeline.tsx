import { useTranslation } from 'react-i18next'
import type { TimelineItem } from './timelineItems'
import { Button } from '../components/Button'
import { partnerLabel } from './familyLabel'
import { CitationText } from '../sources/CitationEditor'
import { formatDate } from '../lib/dates'

interface Props {
  items: TimelineItem[]
  personId: number
  /** Without it the timeline is read-only. */
  onEdit?: (item: TimelineItem) => void
}

export function Timeline({ items, personId, onEdit }: Props) {
  const { t } = useTranslation()
  if (items.length === 0) return <p className="text-slate-600 dark:text-slate-400">{t('event.none')}</p>

  return (
    <ol className="space-y-3">
      {items.map(({ event: e, family }) => {
        const label = e.customLabel || t(`eventType.${e.type}`, { defaultValue: e.type })
        const disproven = e.status === 'disproven'
        return (
          <li
            key={e.id}
            className="flex items-start justify-between gap-3 rounded-xl border border-slate-200 p-3 dark:border-slate-800"
          >
            <div className="min-w-0 space-y-0.5">
              <p className="font-medium">
                <span className={disproven ? 'line-through' : undefined}>{label}</span>
                {e.status !== 'accepted' && (
                  <span
                    className={[
                      'ml-2 rounded px-1.5 py-0.5 text-xs font-semibold',
                      disproven
                        ? 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200'
                        : 'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200',
                    ].join(' ')}
                  >
                    {t(`status.${e.status}`)}
                  </span>
                )}
              </p>
              <p className="text-sm text-slate-700 dark:text-slate-300">
                {e.date.raw ? (e.date.valid ? formatDate(e.date.normalized) : e.date.raw) : t('event.noDate')}
                {e.date.raw && !e.date.valid && <span className="ml-1 italic">({t('date.unrecognizedShort')})</span>}
                {e.place && <> · {e.place.fullName}</>}
              </p>
              {e.description && <p className="text-sm text-slate-700 dark:text-slate-300">{e.description}</p>}
              {family && <p className="text-sm text-slate-600 dark:text-slate-400">{partnerLabel(t, family, personId)}</p>}
              {e.role && <p className="text-sm text-slate-600 dark:text-slate-400">{t('event.asRole', { role: t(`role.${e.role}`, { defaultValue: e.role }) })}</p>}
              {e.status !== 'accepted' && e.statusReason && (
                <p className="text-sm text-slate-600 dark:text-slate-400">{t('event.reason', { reason: e.statusReason })}</p>
              )}
              {e.citations.length > 0 && (
                <ul className="text-sm" aria-label={t('citation.sourcesFor', { what: label })}>
                  {e.citations.map((c) => (
                    <li key={c.citationId}>
                      <span aria-hidden="true">📄 </span>
                      <CitationText citation={c} />
                    </li>
                  ))}
                </ul>
              )}
            </div>
            {onEdit && (
              <Button variant="ghost" onClick={() => onEdit({ event: e, family })}>
                {t('common.edit')}
                <span className="sr-only">: {label}</span>
              </Button>
            )}
          </li>
        )
      })}
    </ol>
  )
}
