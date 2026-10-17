import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getPerson } from '../api/endpoints'
import type { PersonRef, Relation } from '../api/types'
import { Button } from '../components/Button'
import { fullName, lifespan } from '../lib/people'
import { RelativeDialog } from '../people/RelativeDialog'

interface Props {
  person: PersonRef
  isRoot: boolean
  /** Shown as a bottom sheet over the chart (phones and tablets) instead of a side panel. */
  sheet?: boolean
  onCenter: (id: number) => void
  onClose: () => void
}

/** Actions for the person selected in the chart. */
export function PersonPanel({ person, isRoot, sheet = false, onCenter, onClose }: Props) {
  const { t } = useTranslation()
  const [relation, setRelation] = useState<Relation | null>(null)
  const detail = useQuery({ queryKey: ['person', person.id], queryFn: ({ signal }) => getPerson(person.id, signal) })
  const name = fullName(person) ?? t('person.unknown')
  const ref = useRef<HTMLElement>(null)

  // The sheet covers the chart, so keyboard and screen reader users start in it
  // and get back to the card they came from when it closes.
  useEffect(() => {
    if (!sheet) return
    const id = person.id
    ref.current?.focus()
    return () => document.querySelector<HTMLElement>(`[data-tree-person="${id}"]`)?.focus()
  }, [sheet, person.id])

  useEffect(() => {
    const el = ref.current
    if (!sheet || !el) return
    const onKeyDown = (e: globalThis.KeyboardEvent) => {
      // The add-relative dialog lives inside the sheet and handles its own Escape
      // (including the discard prompt), so keys from inside any dialog are ignored.
      if (e.key === 'Escape' && !e.defaultPrevented && !(e.target as Element).closest('dialog')) onClose()
    }
    el.addEventListener('keydown', onKeyDown)
    return () => el.removeEventListener('keydown', onKeyDown)
  }, [sheet, onClose])

  return (
    <section
      ref={ref}
      aria-labelledby="tree-panel-title"
      tabIndex={sheet ? -1 : undefined}
      className={[
        'space-y-3 p-4',
        sheet
          ? 'absolute inset-x-0 bottom-0 z-10 max-h-[70%] overflow-y-auto rounded-t-2xl border-t border-slate-300 bg-white shadow-[0_-8px_24px_rgb(0_0_0/0.18)] outline-none dark:border-slate-600 dark:bg-slate-900'
          : 'rounded-xl border border-slate-200 dark:border-slate-800',
      ].join(' ')}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 id="tree-panel-title" className="text-lg font-semibold">
            {name}
          </h2>
          {lifespan(person) && <p className="text-sm text-slate-600 dark:text-slate-400">{lifespan(person)}</p>}
        </div>
        <Button variant="ghost" onClick={onClose}>
          {t('common.close')}
          <span className="sr-only">: {name}</span>
        </Button>
      </div>
      <div className="flex flex-wrap gap-2">
        {!isRoot && <Button onClick={() => onCenter(person.id)}>{t('tree.centerHere')}</Button>}
        <Link
          to={`/people/${person.id}`}
          className="inline-flex min-h-11 items-center rounded-lg border border-slate-400 px-4 font-medium hover:bg-slate-100 dark:border-slate-600 dark:hover:bg-slate-800"
        >
          {t('tree.openPage')}
        </Link>
      </div>
      <div className="flex flex-wrap gap-2" role="group" aria-label={t('tree.addRelative')}>
        {(['parent', 'partner', 'child', 'sibling'] as const).map((r) => (
          <Button key={r} variant="secondary" disabled={!detail.data} onClick={() => setRelation(r)}>
            + {t(`relative.${r}`)}
          </Button>
        ))}
      </div>
      {detail.data && relation && (
        <RelativeDialog open anchor={detail.data} relation={relation} onClose={() => setRelation(null)} onSaved={() => setRelation(null)} />
      )}
    </section>
  )
}
