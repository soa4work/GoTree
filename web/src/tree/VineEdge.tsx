import type { EdgeProps } from '@xyflow/react'
import { memo, useMemo } from 'react'
import { useDark } from '../lib/theme'
import { VineArt } from './Nature'
import { darkPalette, growVine, lightPalette } from './vine'

/** A connector drawn as a climbing vine; decoration only. */
export const VineEdge = memo(function VineEdge({ id, sourceX, sourceY, targetX, targetY }: EdgeProps) {
  const dark = useDark()
  const vine = useMemo(
    () =>
      sourceY <= targetY
        ? growVine({ x: sourceX, y: sourceY }, { x: targetX, y: targetY }, id)
        : growVine({ x: targetX, y: targetY }, { x: sourceX, y: sourceY }, id),
    [id, sourceX, sourceY, targetX, targetY],
  )
  return (
    <g aria-hidden="true">
      <VineArt vine={vine} palette={dark ? darkPalette : lightPalette} />
    </g>
  )
})
