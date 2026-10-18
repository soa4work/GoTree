import type { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { ApiError, type AuthState } from './api/client'
import { createQueryClient } from './queryClient'

const admin = { id: 1, username: 'admin', displayName: '', role: 'admin' } as NonNullable<AuthState['user']>
const state = (user: AuthState['user']): AuthState => ({
  setupRequired: false,
  user,
  tree: user ? { id: 1, name: 'Weber family', role: 'owner' } : null,
})

async function failMutation(client: QueryClient, err: Error) {
  await client
    .getMutationCache()
    .build(client, { mutationFn: () => Promise.reject(err) })
    .execute(undefined)
    .catch(() => undefined)
}

describe('createQueryClient', () => {
  it('re-checks the session when a mutation gets a 401 while logged in', async () => {
    const client = createQueryClient()
    client.setQueryData(['auth'], state(admin))
    const spy = vi.spyOn(client, 'invalidateQueries')
    await failMutation(client, new ApiError(401, 'unauthorized'))
    expect(spy).toHaveBeenCalledWith({ queryKey: ['auth'] })
  })

  it('ignores a 401 when nobody is logged in (wrong password)', async () => {
    const client = createQueryClient()
    client.setQueryData(['auth'], state(null))
    const spy = vi.spyOn(client, 'invalidateQueries')
    await failMutation(client, new ApiError(401, 'wrong password'))
    expect(spy).not.toHaveBeenCalled()
  })

  it.each(['not_member', 'view_only'])('re-checks the auth state when a mutation gets a 403 %s', async (code) => {
    const client = createQueryClient()
    client.setQueryData(['auth'], state(admin))
    const spy = vi.spyOn(client, 'invalidateQueries')
    await failMutation(client, new ApiError(403, 'no', {}, { code }))
    expect(spy).toHaveBeenCalledWith({ queryKey: ['auth'] })
  })

  it('ignores a 403 with another code', async () => {
    const client = createQueryClient()
    client.setQueryData(['auth'], state(admin))
    const spy = vi.spyOn(client, 'invalidateQueries')
    await failMutation(client, new ApiError(403, 'no', {}, { code: 'owner_only' }))
    expect(spy).not.toHaveBeenCalled()
  })

  it('ignores other errors', async () => {
    const client = createQueryClient()
    client.setQueryData(['auth'], state(admin))
    const spy = vi.spyOn(client, 'invalidateQueries')
    await failMutation(client, new ApiError(500, 'boom'))
    expect(spy).not.toHaveBeenCalled()
  })
})
