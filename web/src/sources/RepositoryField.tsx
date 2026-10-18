import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createRepository, listRepositories } from '../api/endpoints'
import type { Repository } from '../api/types'
import { Button } from '../components/Button'
import { SearchSelect } from '../components/SearchSelect'
import { errorText } from '../lib/errors'

export function RepositoryField({ value, onChange }: { value: Repository | null; onChange: (r: Repository | null) => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const create = useMutation({
    mutationFn: createRepository,
    onSuccess: (r) => {
      void queryClient.invalidateQueries({ queryKey: ['repositories'] })
      onChange(r)
    },
  })
  return (
    <SearchSelect<Repository>
      label={t('source.repository')}
      hint={t('source.repositoryHint')}
      queryKey="repositories"
      search={(q, signal) => listRepositories(q, signal)}
      getKey={(r) => r.id}
      itemText={(r) => r.name}
      renderItem={(r) => r.name}
      value={value}
      onChange={onChange}
      footer={(q) => (
        <>
          {q && (
            <Button type="button" variant="ghost" className="mt-2" busy={create.isPending} onClick={() => create.mutate(q)}>
              {t('source.createRepository', { name: q })}
            </Button>
          )}
          {create.isError && (
            <p role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
              {t('common.saveFailed', { message: errorText(create.error, t) })}
            </p>
          )}
        </>
      )}
    />
  )
}
