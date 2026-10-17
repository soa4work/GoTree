import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { parseDate } from '../api/endpoints'
import { TextField } from '../components/TextField'
import { useDebounced } from '../hooks/useDebounced'
import { formatDate } from '../lib/dates'

interface Props {
  value: string
  onChange: (value: string) => void
  error?: string
}

/** A free-text genealogical date with a live preview of how it is stored. */
export function DateField({ value, onChange, error }: Props) {
  const { t } = useTranslation()
  const q = useDebounced(value.trim(), 300)
  const parsed = useQuery({
    queryKey: ['date-parse', q],
    queryFn: ({ signal }) => parseDate(q, signal),
    enabled: q !== '',
    staleTime: Infinity,
  })

  let preview: string = t('date.hint')
  if (q !== '' && parsed.data) {
    preview = parsed.data.valid ? t('date.storedAs', { date: formatDate(parsed.data.normalized) }) : t('date.unrecognized')
  }

  return (
    <TextField
      label={t('event.date')}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      error={error}
      hint={preview}
      autoComplete="off"
      inputMode="text"
    />
  )
}
