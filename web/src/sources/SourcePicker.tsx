import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createSource, listSources } from '../api/endpoints'
import type { Source } from '../api/types'
import { Button } from '../components/Button'
import { SearchSelect } from '../components/SearchSelect'
import { errorText } from '../lib/errors'

interface Props {
  value: Source | null
  onChange: (s: Source | null) => void
  error?: string
}

/** Search the sources, or create one from the typed title. */
export function SourcePicker({ value, onChange, error }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const create = useMutation({
    mutationFn: (title: string) =>
      createSource({ title, author: '', publication: '', callNumber: '', repositoryId: null, notes: '' }),
    onSuccess: (s) => {
      void queryClient.invalidateQueries({ queryKey: ['sources'] })
      onChange(s)
    },
  })
  return (
    <SearchSelect<Source>
      label={t('citation.source')}
      hint={t('citation.sourceHint')}
      queryKey="sources"
      search={(q, signal) => listSources(q, signal)}
      getKey={(s) => s.id}
      itemText={(s) => s.title}
      renderItem={(s) => (
        <>
          <span className="font-medium">{s.title}</span>
          {s.author && <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">{s.author}</span>}
        </>
      )}
      value={value}
      onChange={onChange}
      error={error ?? (create.error ? t('common.saveFailed', { message: errorText(create.error, t) }) : undefined)}
      footer={(q) =>
        q && (
          <Button type="button" variant="ghost" className="mt-2" busy={create.isPending} onClick={() => create.mutate(q)}>
            {t('citation.createSource', { title: q })}
          </Button>
        )
      }
    />
  )
}
