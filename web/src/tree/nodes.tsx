import { Handle, Position, type Node, type NodeProps } from '@xyflow/react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import type { PersonRef } from '../api/types'
import { fullName, lifespan } from '../lib/people'
import { Avatar } from '../media/Avatar'
import { HEART_PATH } from './vine'
import type { TreeLook } from './nodeTypes'
import { useTreeActions, useTreeUi } from './treeUi'

export interface PersonNodeData extends Record<string, unknown> {
  person: PersonRef
  root: boolean
  look: TreeLook
}

export interface LookData extends Record<string, unknown> {
  look?: TreeLook
}

export interface RepeatNodeData extends LookData {
  person: PersonRef
}

const hidden = { opacity: 0, pointerEvents: 'none' as const }

// React Flow disables pointer events on nodes it considers inert (we turn
// off its own selection); the cards are buttons and must stay clickable.
const interactive = { pointerEvents: 'auto' as const }

function Handles() {
  return (
    <>
      <Handle type="target" position={Position.Top} isConnectable={false} style={hidden} />
      <Handle type="source" position={Position.Bottom} isConnectable={false} style={hidden} />
    </>
  )
}

const sexMark: Record<string, string> = { M: '♂', F: '♀', X: '⚧', U: '' }
const accent: Record<string, string> = {
  M: 'bg-sky-600',
  F: 'bg-rose-500',
  X: 'bg-violet-500',
}

export const PersonNode = memo(function PersonNode({ data }: NodeProps<Node<PersonNodeData>>) {
  const { t } = useTranslation()
  const p = data.person
  const selected = useTreeUi((s) => s.selectedId === p.id)
  const tabbable = useTreeUi((s) => s.focusId === p.id)
  const a = useTreeActions()
  const given = p.givenNames.trim()
  const surname = p.surname.trim()
  const years = lifespan(p)
  const leafy = data.look === 'leafy'
  return (
    <>
      <Handles />
      <button
        type="button"
        data-tree-person={p.id}
        tabIndex={tabbable ? 0 : -1}
        aria-pressed={selected}
        aria-current={data.root ? 'true' : undefined}
        onClick={() => a.select(p.id)}
        onDoubleClick={() => a.center(p.id)}
        onKeyDown={(e) => a.key(p.id, e)}
        style={interactive}
        className={[
          'nodrag relative flex h-16 w-[180px] items-center gap-2 pr-2 text-left transition-[box-shadow,translate] duration-150 hover:-translate-y-px',
          leafy
            ? [
                'rounded-full pl-2 hover:brightness-105',
                'bg-linear-to-b shadow-[inset_0_0_0_3px_rgb(255_253_245),inset_0_0_0_4px_rgb(201_180_143)] dark:shadow-[inset_0_0_0_3px_rgb(26_22_16),inset_0_0_0_4px_rgb(110_88_60)]',
                data.root
                  ? 'border-[3px] border-amber-700 from-amber-50 to-[#f6e7c4] dark:border-amber-400 dark:from-[#2e2412] dark:to-[#1f180c]'
                  : 'border-2 border-[#7a5534] from-[#fffdf5] to-[#f3ead3] dark:border-[#a98260] dark:from-[#221c14] dark:to-[#18140e]',
              ].join(' ')
            : [
                'overflow-hidden rounded-2xl pl-3.5 shadow-md shadow-slate-900/10 hover:shadow-lg dark:shadow-black/40',
                data.root
                  ? 'border-2 border-brand-700 bg-linear-to-br from-brand-50 to-white dark:border-brand-100 dark:from-brand-800 dark:to-slate-900'
                  : 'border border-slate-300 bg-white dark:border-slate-600 dark:bg-slate-900',
              ].join(' '),
          selected ? 'ring-4 ring-brand-500/60' : '',
        ].join(' ')}
      >
        {!leafy && accent[p.sex] && (
          <span aria-hidden="true" className={`absolute inset-y-0 left-0 w-1.5 ${accent[p.sex]}`} />
        )}
        <span
          className={`rounded-full ring-2 ${leafy ? 'ring-[#c9b48f] dark:ring-[#6e583c]' : 'ring-white dark:ring-slate-800'}`}
        >
          <Avatar person={p} size={40} />
        </span>
        <span className="flex min-w-0 flex-col leading-tight">
          <span className="truncate text-[13px] text-slate-800 dark:text-slate-200">
            {sexMark[p.sex] && (
              <span aria-hidden="true" className="mr-1 text-slate-600 dark:text-slate-300">
                {sexMark[p.sex]}
              </span>
            )}
            {given || (!surname && t('person.unknown'))}
          </span>{' '}
          {surname && (
            <span className="truncate text-sm font-bold tracking-wide text-slate-900 dark:text-white">{surname}</span>
          )}
          <span className="truncate text-xs text-slate-600 tabular-nums dark:text-slate-300">
            {years}
            <span className="sr-only">
              {', '}
              {t(`sex.${p.sex}`)}
              {data.root ? `, ${t('tree.rootPerson')}` : ''}
            </span>
          </span>
        </span>
      </button>
    </>
  )
})

export const JunctionNode = memo(function JunctionNode({ data }: NodeProps<Node<LookData>>) {
  if (data.look === 'leafy') {
    return (
      <>
        <Handles />
        <svg aria-hidden="true" viewBox="-7 -7 14 14" className="size-3 overflow-visible">
          <path d={HEART_PATH} transform="scale(1.3)" className="fill-rose-700 dark:fill-rose-400" />
        </svg>
      </>
    )
  }
  return (
    <>
      <Handles />
      <div
        aria-hidden="true"
        className="size-3 rounded-full border-[3px] border-slate-500 bg-white dark:border-slate-400 dark:bg-slate-950"
      />
    </>
  )
})

export const UnknownNode = memo(function UnknownNode({ data }: NodeProps<Node<LookData>>) {
  const { t } = useTranslation()
  return (
    <>
      <Handles />
      <div
        className={`flex h-12 w-[180px] items-center justify-center border-2 border-dashed text-sm italic text-slate-600 dark:text-slate-300 ${
          data.look === 'leafy'
            ? 'rounded-full border-[#4d8a3e] bg-[#fffdf5]/80 dark:border-[#86b55a] dark:bg-[#14201a]/80'
            : 'rounded-2xl border-slate-400 bg-slate-50/80 dark:border-slate-600 dark:bg-slate-900/80'
        }`}
      >
        {t('person.unknownParent')}
      </div>
    </>
  )
})

export const RepeatNode = memo(function RepeatNode({ data }: NodeProps<Node<RepeatNodeData>>) {
  const { t } = useTranslation()
  const a = useTreeActions()
  const name = fullName(data.person) ?? t('person.unknown')
  return (
    <>
      <Handles />
      <button
        type="button"
        tabIndex={-1}
        onClick={() => a.jump(data.person.id)}
        style={interactive}
        className={`nodrag flex h-12 w-[132px] items-center ${data.look === 'leafy' ? 'rounded-full px-4' : 'rounded-2xl px-2'} border-2 border-dashed border-brand-500 bg-brand-50 text-left text-xs text-slate-700 shadow-sm dark:bg-slate-900 dark:text-slate-200`}
      >
        <span className="line-clamp-2">
          <span aria-hidden="true">↺ </span>
          {t('tree.repeat', { name })}
        </span>
      </button>
    </>
  )
})
