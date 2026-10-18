import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createPlace, listPlaces } from '../api/endpoints'
import type { Place, PlaceRef } from '../api/types'
import { Button } from '../components/Button'
import { SearchSelect } from '../components/SearchSelect'
import { errorText } from '../lib/errors'

interface Props {
  value: PlaceRef | null
  onChange: (place: PlaceRef | null) => void
  error?: string
}

/**
 * Search the place list, or create the typed place on the spot. A comma
 * in the new name builds the hierarchy: "Potsdam, Brandenburg, Germany"
 * becomes three nested places, reusing any that already exist.
 */
export function PlaceField({ value, onChange, error }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const create = useMutation({
    mutationFn: async (fullName: string) => {
      const parts = fullName
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean)
        .reverse()
      let parentId: number | null = null
      let created: PlaceRef | null = null
      for (const name of parts) {
        const existing = (await listPlaces(name)).find((p) => p.name === name && p.parentId === parentId)
        const place: Place = existing ?? (await createPlace({ parentId, name, placeType: '', lat: null, lng: null, notes: '' }))
        parentId = place.id
        created = { id: place.id, fullName: place.fullName }
      }
      return created
    },
    onSuccess: (place) => {
      void queryClient.invalidateQueries({ queryKey: ['places'] })
      onChange(place)
    },
  })

  return (
    <SearchSelect<PlaceRef>
      label={t('event.place')}
      hint={t('place.hint')}
      queryKey="places"
      search={(q, signal) => listPlaces(q, signal)}
      getKey={(p) => p.id}
      itemText={(p) => p.fullName}
      renderItem={(p) => p.fullName}
      value={value}
      onChange={onChange}
      error={error ?? (create.error ? t('place.createFailed', { message: errorText(create.error, t) }) : undefined)}
      footer={(q) =>
        q && (
          <Button type="button" variant="ghost" className="mt-2" busy={create.isPending} onClick={() => create.mutate(q)}>
            {t('place.create', { name: q })}
          </Button>
        )
      }
    />
  )
}
