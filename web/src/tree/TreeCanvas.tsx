import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  getViewportForBounds,
  useReactFlow,
  useStoreApi,
  type Edge,
  type Node,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useEffect, useId, useMemo, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useDark } from '../lib/theme'
import type { LayoutNode, TreeLayout } from './layout'
import type { TreeIndex } from './model'
import { edgeTypes, nodeTypes, type TreeLook } from './nodeTypes'
import type { PersonNodeData, RepeatNodeData } from './nodes'

interface Props {
  layout: TreeLayout
  index: TreeIndex
  selectedId: number | null
  onSelect: (id: number) => void
  onCenter: (id: number) => void
  look?: TreeLook
  /** A bottom sheet covers the lower part of the canvas. */
  sheetOpen?: boolean
}

export function TreeCanvas(props: Props) {
  return (
    <ReactFlowProvider>
      <Canvas {...props} />
    </ReactFlowProvider>
  )
}

/** Below this zoom the 13px names on the cards become hard to read. */
const readableZoom = 0.8

const center = (n: LayoutNode) => ({ x: n.x + n.w / 2, y: n.y + n.h / 2 })

/** The nearest person card in an arrow key's direction, preferring the same column or row. */
function neighbour(nodes: LayoutNode[], from: LayoutNode, key: string): LayoutNode | undefined {
  const c = center(from)
  let best: LayoutNode | undefined
  let bestScore = Infinity
  for (const n of nodes) {
    if (n.kind !== 'person' || n.id === from.id) continue
    const d = center(n)
    const dx = d.x - c.x
    const dy = d.y - c.y
    let ok: boolean
    let score: number
    switch (key) {
      case 'ArrowUp':
        ok = dy < -1
        score = -dy * 1000 + Math.abs(dx)
        break
      case 'ArrowDown':
        ok = dy > 1
        score = dy * 1000 + Math.abs(dx)
        break
      case 'ArrowLeft':
        ok = Math.abs(dy) < 1 && dx < 0
        score = -dx
        break
      default:
        ok = Math.abs(dy) < 1 && dx > 0
        score = dx
    }
    if (ok && score < bestScore) {
      best = n
      bestScore = score
    }
  }
  return best
}

function Canvas({ layout, index, selectedId, onSelect, onCenter, look = 'leafy', sheetOpen = false }: Props) {
  const { t } = useTranslation()
  const flow = useReactFlow()
  const store = useStoreApi()
  const dark = useDark()
  const helpId = useId()
  const rootId = index.rootId
  const [focusId, setFocusId] = useState<number>(rootId)
  const [prevRoot, setPrevRoot] = useState(rootId)
  // A new root moves the single tab stop back to it.
  if (prevRoot !== rootId) {
    setPrevRoot(rootId)
    setFocusId(rootId)
  }

  const byPerson = useMemo(
    () => new Map(layout.nodes.filter((n) => n.kind === 'person').map((n) => [n.personId!, n])),
    [layout],
  )

  const reveal = (id: number, focus: boolean) => {
    const n = byPerson.get(id)
    if (!n) return
    const c = center(n)
    void flow.setCenter(c.x, c.y, { zoom: Math.max(flow.getZoom(), 0.8), duration: 250 })
    setFocusId(id)
    if (focus) requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-tree-person="${id}"]`)?.focus())
  }

  const onKey = (id: number, e: KeyboardEvent<HTMLButtonElement>) => {
    if (!e.key.startsWith('Arrow')) return
    const from = byPerson.get(id)
    const to = from && neighbour(layout.nodes, from, e.key)
    e.preventDefault()
    if (to) reveal(to.personId!, true)
  }

  const nodes: Node[] = useMemo(
    () =>
      layout.nodes.map((n): Node => {
        const base = { id: n.id, position: { x: n.x, y: n.y }, width: n.w, height: n.h, type: n.kind }
        if (n.kind === 'person') {
          const data: PersonNodeData = {
            person: index.persons.get(n.personId!)!,
            root: n.personId === rootId,
            selected: n.personId === selectedId,
            tabbable: n.personId === focusId,
            onSelect: (id) => {
              setFocusId(id)
              onSelect(id)
            },
            onCenter,
            onKey,
            look,
          }
          return { ...base, data }
        }
        if (n.kind === 'repeat') {
          const data: RepeatNodeData = {
            person: index.persons.get(n.personId!)!,
            onJump: (id) => reveal(id, true),
            look,
          }
          return { ...base, data }
        }
        return { ...base, data: { look } }
      }),
    // onKey and reveal only read layout-derived values that are deps here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [layout, index, selectedId, focusId, rootId, onSelect, onCenter, look],
  )
  const edges: Edge[] = useMemo(() => {
    if (look === 'leafy') return layout.edges.map((e) => ({ ...e, type: 'vine', focusable: false }))
    return layout.edges.map((e) => ({
      ...e,
      type: 'smoothstep',
      pathOptions: { borderRadius: 14 },
      focusable: false,
      style: { strokeWidth: 2, stroke: dark ? '#a59a82' : '#7b715f' },
    }))
  }, [layout, dark, look])
  const leafy = look === 'leafy'

  // Re-frame the chart whenever a different tree is shown: all of it if the
  // names stay readable, otherwise the root at a readable zoom.
  useEffect(() => {
    const id = requestAnimationFrame(() => {
      const { width, height } = store.getState()
      const person = layout.nodes.filter((n) => n.kind === 'person')
      if (!width || !height || person.length === 0) {
        void flow.fitView({ padding: 0.15, maxZoom: 1, duration: 200 })
        return
      }
      const x1 = Math.min(...layout.nodes.map((n) => n.x))
      const y1 = Math.min(...layout.nodes.map((n) => n.y))
      const x2 = Math.max(...layout.nodes.map((n) => n.x + n.w))
      const y2 = Math.max(...layout.nodes.map((n) => n.y + n.h))
      const fit = getViewportForBounds({ x: x1, y: y1, width: x2 - x1, height: y2 - y1 }, width, height, 0.1, 1, 0.15)
      if (fit.zoom >= readableZoom) {
        void flow.setViewport(fit, { duration: 200 })
        return
      }
      const start = byPerson.get(rootId) ?? person[0]
      if (!start) return
      const c = center(start)
      void flow.setCenter(c.x, c.y, { zoom: readableZoom, duration: 200 })
    })
    return () => cancelAnimationFrame(id)
    // byPerson and rootId follow layout.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layout, flow, store])

  // The sheet hides the lower part of the canvas: move the selected card up
  // into the visible part.
  useEffect(() => {
    const n = selectedId === null ? undefined : byPerson.get(selectedId)
    if (!sheetOpen || !n) return
    const id = requestAnimationFrame(() => {
      const sheet = document.getElementById('tree-panel-title')?.closest('section')
      const zoom = Math.max(flow.getZoom(), readableZoom)
      const c = center(n)
      void flow.setCenter(c.x, c.y + (sheet ? sheet.offsetHeight / 2 : 0) / zoom, { zoom, duration: 250 })
    })
    return () => cancelAnimationFrame(id)
    // byPerson follows layout.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, sheetOpen, flow])

  return (
    <div role="group" aria-label={t('tree.chartLabel')} aria-describedby={helpId} className="size-full">
      <p id={helpId} className="sr-only">
        {t('tree.chartHelp')}
      </p>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        nodesDraggable={false}
        nodesConnectable={false}
        nodesFocusable={false}
        edgesFocusable={false}
        elementsSelectable={false}
        minZoom={0.1}
        maxZoom={2}
        zoomOnDoubleClick={false}
        onlyRenderVisibleElements={layout.nodes.length > 300}
        colorMode={dark ? 'dark' : 'light'}
        disableKeyboardA11y
      >
        <Background
          bgColor={leafy ? (dark ? '#0b140f' : '#f7f3e6') : dark ? '#0f1510' : '#faf7ef'}
          gap={20}
          size={1.5}
          color={leafy ? (dark ? '#1f3a26' : '#d9d0b4') : dark ? '#2a3128' : '#cdc3ad'}
        />
        <Controls showInteractive={false} position="bottom-left" />
        <MiniMap
          className="!hidden lg:!block"
          style={{
            width: 150,
            height: 96,
            backgroundColor: dark ? '#1b221c' : '#faf7ef',
            border: `1px solid ${dark ? '#463e31' : '#cdc3ad'}`,
            borderRadius: 8,
          }}
          position="bottom-right"
          pannable
          zoomable
          ariaLabel={t('tree.minimap')}
          nodeBorderRadius={8}
          nodeColor={(n) => (n.type === 'person' ? (dark ? '#7b715f' : '#a59a82') : 'transparent')}
          maskColor={dark ? 'rgb(15 21 16 / 0.6)' : 'rgb(242 237 224 / 0.6)'}
        />
      </ReactFlow>
    </div>
  )
}
