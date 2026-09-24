export type ApiErrorBody = { code?: string; message?: string; details?: Record<string, unknown> }

export class ApiError extends Error {
  code: string
  details: Record<string, unknown>
  status: number
  constructor(status: number, body: ApiErrorBody) {
    super(body.message || `HTTP ${status}`)
    this.status = status
    this.code = body.code || 'UNKNOWN'
    this.details = body.details || {}
  }
}

const base = typeof document === 'undefined' ? '/' : document.querySelector('base')?.getAttribute('href') || '/'

/** Builds an absolute URL that respects the injected <base href>. */
export function endpoint(path: string) {
  return `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const method = (init.method || 'GET').toUpperCase()
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && !headers.has('X-Requested-With')) {
    headers.set('X-Requested-With', 'fetch')
  }
  if (init.body && typeof init.body === 'string' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(endpoint(path), { credentials: 'include', ...init, headers })
  const raw = await response.text()
  let body: unknown = {}
  if (raw) {
    try {
      body = JSON.parse(raw)
    } catch {
      body = { message: raw }
    }
  }
  if (!response.ok) {
    throw new ApiError(response.status, body as ApiErrorBody)
  }
  return body as T
}

export const api = {
  me: () => request<{ user: User; insecure: boolean }>('api/v1/auth/me'),
  setupStatus: () => request<{ required: boolean; insecure: boolean; site_name?: string }>('api/v1/setup/status'),
  login: (username: string, password: string) => request('api/v1/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  setup: (username: string, password: string) => request('api/v1/setup', { method: 'POST', body: JSON.stringify({ username, password }) }),
  logout: () => request('api/v1/auth/logout', { method: 'POST' }),
  changePassword: (CurrentPassword: string, Password: string) => request('api/v1/auth/password', { method: 'POST', body: JSON.stringify({ CurrentPassword, Password }) }),
  settings: () => request<Record<string, string>>('api/v1/settings'),
  saveSettings: (values: Record<string, string>) => request('api/v1/settings', { method: 'PUT', body: JSON.stringify(values) }),
  users: () => request<User[]>('api/v1/users'),
  createUser: (body: unknown) => request('api/v1/users', { method: 'POST', body: JSON.stringify(body) }),
  updateUser: (id: number, body: unknown) => request(`api/v1/users/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteUser: (id: number) => request(`api/v1/users/${id}`, { method: 'DELETE' }),
  resetPassword: (id: number, password: string) => request(`api/v1/users/${id}/password`, { method: 'POST', body: JSON.stringify({ password }) }),
  usageOverview: () => request<UsageOverview>('api/v1/usage'),
  usageMe: () => request<{ root_dir: string; used: number; quota: number }>('api/v1/usage/me'),
  recalculate: () => request('api/v1/usage/recalculate', { method: 'POST' }),
  list: (path: string, userId?: string) => request<{ entries: Entry[] }>(`api/v1/files?${fileQuery({ path }, userId)}`),
  fileAction: (method: string, body: unknown, userId?: string) =>
    request(`api/v1/files/action${userId ? `?user_id=${encodeURIComponent(userId)}` : ''}`, { method, body: JSON.stringify(body) })
}

export type User = {
  id: number
  username: string
  role: string
  root_dir: string
  permission: string
  quota: number
  disabled: boolean
  used?: number
}

export type Entry = { name: string; directory: boolean; size: number; modified: string }
export type UsageOverview = {
  users: { user: User; used: number }[]
  disk: { storage_dir?: string; free?: number; total?: number }
}

export function fileQuery(params: { path?: string; userId?: string }, userId?: string) {
  const search = new URLSearchParams()
  if (params.path) search.set('path', params.path)
  const target = userId || params.userId
  if (target) search.set('user_id', target)
  return search.toString()
}

export function rawURL(path: string, userId?: string) {
  return endpoint(`api/v1/files/raw?${fileQuery({ path }, userId)}`)
}

export function downloadURL(paths: string[], userId?: string) {
  const search = new URLSearchParams()
  for (const path of paths.length ? paths : ['']) search.append('path', path)
  if (userId) search.set('user_id', userId)
  return endpoint(`api/v1/files/download?${search.toString()}`)
}
