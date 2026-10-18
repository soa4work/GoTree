import type { TFunction } from 'i18next'
import type { ReportLine, TagStat } from '../api/types'

// Keys are built from server codes at runtime, so they cannot be checked against the typed resources.
type LooseT = (key: string, options?: Record<string, unknown>) => string

/**
 * A line of an import or check report in the interface language. The server
 * sends a code and params; a line without a code, or with one this version
 * does not know, shows the English message it came with.
 */
export function reportLineText(t: TFunction, w: ReportLine): string {
  const loose = t as unknown as LooseT
  let text = w.message
  if (w.code) {
    const params: Record<string, unknown> = { ...w.params }
    // "what" names the kind of record a reference points at (transfer.refKind.*).
    if (typeof params.what === 'string') params.what = loose(`transfer.refKind.${params.what}`, { defaultValue: params.what })
    // A numeric "count" param selects the plural form.
    text = loose(`transfer.warning.${w.code}`, { ...params, defaultValue: w.message })
  }
  return w.line ? t('transfer.atLine', { line: w.line, message: text }) : text
}

/** Why a tag was dropped, in the interface language. */
export function dropReasonText(t: TFunction, tag: Pick<TagStat, 'reason' | 'reasonCode'>): string {
  const fallback = tag.reason ?? ''
  if (!tag.reasonCode) return fallback
  return (t as unknown as LooseT)(`transfer.dropReason.${tag.reasonCode}`, { defaultValue: fallback })
}
