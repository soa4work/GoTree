import { useQuery } from '@tanstack/react-query'
import { getUnmappedPlaces } from '../api/endpoints'

export const unmappedPlacesKey = ['map-unmapped-places']

export const useUnmappedPlaces = () =>
  useQuery({
    queryKey: unmappedPlacesKey,
    queryFn: ({ signal }) => getUnmappedPlaces(signal),
  })
