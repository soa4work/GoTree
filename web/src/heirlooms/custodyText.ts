import type { TFunction } from 'i18next'
import type { Custody } from '../api/types'
import { formatDate } from '../lib/dates'

/** "1920 – 1965", "since 1965" or "" for a custody entry. */
export function custodyPeriod(t: TFunction, c: Pick<Custody, 'fromDate' | 'toDate'>): string {
  if (c.fromDate && c.toDate) return t('heirloom.period', { from: formatDate(c.fromDate), to: formatDate(c.toDate) })
  if (c.fromDate) return t('heirloom.since', { from: formatDate(c.fromDate, 'since') })
  if (c.toDate) return t('heirloom.until', { to: formatDate(c.toDate, 'until') })
  return ''
}
