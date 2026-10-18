import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { switchTree } from '../api/client'

/** Switches the session to another tree and drops everything loaded from the old one. */
export function useSwitchTree() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  return useMutation({
    mutationFn: switchTree,
    onSuccess: async (state) => {
      // No query key contains the tree id, so data of the old tree would
      // otherwise show under the new one. The auth state and the list of the
      // user's own trees do not depend on the current tree.
      const ofTree = (q: { queryKey: readonly unknown[] }) => q.queryKey[0] !== 'auth' && q.queryKey[0] !== 'trees'
      await queryClient.cancelQueries({ predicate: ofTree })
      queryClient.removeQueries({ predicate: ofTree })
      queryClient.setQueryData(['auth'], state)
      void queryClient.invalidateQueries({ queryKey: ['trees'] })
      navigate('/')
    },
  })
}
