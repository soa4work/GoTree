import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { ApiError, type AuthState } from './api/client'

export function createQueryClient(): QueryClient {
  // An expired session surfaces as a 401 from any query or mutation;
  // re-checking the auth state brings up the login screen. A 403 that says
  // the user left the tree or lost the right to write means the cached role
  // is out of date, and the same re-check shows the tree picker or the new role.
  const onAuthError = (err: unknown) => {
    if (!(err instanceof ApiError)) return
    const stale = err.status === 401 || (err.status === 403 && (err.code === 'not_member' || err.code === 'view_only'))
    if (!stale) return
    // A wrong password on the login form is a 401 too; only a lost session counts.
    if (!client.getQueryData<AuthState>(['auth'])?.user) return
    void client.invalidateQueries({ queryKey: ['auth'] })
  }
  const client: QueryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 1,
      },
    },
    queryCache: new QueryCache({ onError: onAuthError }),
    mutationCache: new MutationCache({ onError: onAuthError }),
  })
  return client
}
