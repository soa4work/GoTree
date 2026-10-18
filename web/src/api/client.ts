export type ErrorParams = Record<string, string | number>

/** Machine-readable part of an error: a stable code and its message parameters. */
export interface ErrorDetail {
  code?: string
  params?: ErrorParams
}

/** The JSON body of an API error response; `error` is the English message. */
interface ErrorBody extends ErrorDetail {
  error?: string
  fields?: Record<string, string>
  fieldCodes?: Record<string, ErrorDetail>
}

export class ApiError extends Error {
  readonly status: number
  readonly fields: Record<string, string>
  /** Stable server error code, e.g. "gedcom_too_large"; undefined for old servers and proxies. */
  readonly code?: string
  readonly params: ErrorParams
  readonly fieldCodes: Record<string, ErrorDetail>

  constructor(status: number, message: string, fields: Record<string, string> = {}, body: ErrorBody = {}) {
    super(message)
    this.status = status
    this.fields = fields
    this.code = body.code
    this.params = body.params ?? {}
    this.fieldCodes = body.fieldCodes ?? {}
  }
}

async function request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(`/api${path}`, {
    method,
    signal,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    let message = res.statusText
    let fields: Record<string, string> = {}
    let body: ErrorBody = {}
    try {
      body = (await res.json()) as ErrorBody
      message = body.error ?? message
      fields = body.fields ?? {}
    } catch {
      // Non-JSON error bodies (e.g. a proxy page) keep the status text.
    }
    throw new ApiError(res.status, message, fields, body)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** Uploads one file as multipart form data ("file"). */
export async function uploadFile<T>(path: string, file: File): Promise<T> {
  const body = new FormData()
  body.append('file', file)
  const res = await fetch(`/api${path}`, { method: 'POST', body, headers: { Accept: 'application/json' } })
  if (!res.ok) {
    let message = res.statusText
    let body: ErrorBody = {}
    try {
      body = (await res.json()) as ErrorBody
      message = body.error ?? message
    } catch {
      // keep the status text
    }
    throw new ApiError(res.status, message, {}, body)
  }
  return (await res.json()) as T
}

export const getJSON = <T>(path: string, signal?: AbortSignal) => request<T>('GET', path, undefined, signal)
export const postJSON = <T>(path: string, body?: unknown) => request<T>('POST', path, body)
export const putJSON = <T>(path: string, body: unknown) => request<T>('PUT', path, body)
export const del = (path: string) => request<void>('DELETE', path)

export interface Health {
  status: 'ok' | 'degraded'
  version: string
  database: 'ok' | 'unreachable'
}

export const getHealth = (signal?: AbortSignal) => getJSON<Health>('/health', signal)

export interface User {
  id: number
  username: string
  displayName: string
  role: 'admin' | 'user'
}

export interface AuthState {
  setupRequired: boolean
  user: User | null
  tree: { id: number; name: string; role: 'owner' | 'editor' | 'viewer' } | null
}

export const getAuthState = (signal?: AbortSignal) => getJSON<AuthState>('/auth/state', signal)

export interface SetupInput {
  username: string
  password: string
  treeName: string
}

export const setup = (input: SetupInput) => postJSON<AuthState>('/auth/setup', input)
export const login = (username: string, password: string) => postJSON<void>('/auth/login', { username, password })
export const logout = () => postJSON<void>('/auth/logout')
