import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, logout } from '../api/client'

/** Logs out and drops everything the previous user loaded; the auth refetch then shows the login screen. */
export function useLogout() {
  const queryClient = useQueryClient()
  const signedOut = () => {
    // Keep the auth query mounted so its refetch switches to the login screen.
    queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== 'auth' })
    void queryClient.invalidateQueries({ queryKey: ['auth'] })
  }
  return useMutation({
    mutationFn: logout,
    onSuccess: () => signedOut(),
    // The session was already gone, so the user is effectively logged out.
    onError: (err) => {
      if (err instanceof ApiError && err.status === 401) signedOut()
    },
  })
}
