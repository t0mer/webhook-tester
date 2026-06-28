// Typed client for the Raptor management API. Every UI action maps to a
// documented /api/v1 endpoint.

export interface Token {
  uuid: string
  alias?: string
  url: string
  default_status: number
  default_content: string
  default_content_type: string
  timeout: number
  cors: boolean
  expiry: number
  actions: boolean
  request_limit: number
  description: string
  listen: number
  redirect: string
  group_id?: string
  premium: boolean
  has_password: boolean
  created_at: string
  updated_at: string
  latest_request_at?: string | null
}

export interface RequestFile {
  id: string
  request_id: string
  filename: string
  content_type: string
  size: number
}

export interface CapturedRequest {
  uuid: string
  token_id: string
  type: string
  method: string
  ip: string
  hostname: string
  user_agent: string
  content: string
  query: Record<string, string[]> | null
  headers: Record<string, string[]> | null
  url: string
  size: number
  sorting: number
  files?: RequestFile[]
  created_at: string
  custom_action_output?: Record<string, unknown>
  custom_action_errors?: Record<string, unknown>
  // Email-specific (type === 'email')
  sender?: string
  message_id?: string
  destinations?: string
  subject?: string
  text_content?: string
  checks?: Record<string, string>
}

export interface RequestPage {
  data: CapturedRequest[]
  total: number
  page: number
  per_page: number
}

export interface Group {
  id: string
  name: string
  color?: string
  created_at: string
}

export interface Action {
  uuid: string
  token_id: string
  type: string
  name: string
  position: number
  disabled: boolean
  parameters: Record<string, unknown>
}

export interface ActionRun {
  id: string
  action_type: string
  action_name: string
  position: number
  output: string
  error: string
  created_at: string
}

export interface ActionInput {
  type: string
  name?: string
  disabled?: boolean
  parameters?: Record<string, unknown>
}

export interface User {
  id: string
  email: string
  role: string
  created_at: string
  updated_at: string
}

export interface AuthStatus {
  bootstrapped: boolean
  require_auth: boolean
  authenticated: boolean
  user?: User
}

export interface APIKey {
  id: string
  user_id: string
  name: string
  last_used_at?: string | null
  created_at: string
}

export interface Schedule {
  uuid: string
  token_id?: string
  name: string
  cron: string
  target_url: string
  method: string
  body: string
  run_actions: boolean
  expect_status: number
  keyword: string
  check_ssl: boolean
  ssl_days: number
  has_notify: boolean
  enabled: boolean
  last_run?: string | null
  next_run?: string | null
  last_status: string
  last_message: string
}

export type ScheduleInput = Partial<{
  token_id: string
  name: string
  cron: string
  target_url: string
  method: string
  body: string
  run_actions: boolean
  expect_status: number
  keyword: string
  check_ssl: boolean
  ssl_days: number
  notify_url: string
  enabled: boolean
}>

export interface ScheduleRun {
  id: string
  schedule_id: string
  status: string
  status_code: number
  message: string
  duration_ms: number
  created_at: string
}

export interface TestActionResult {
  output: string
  error: string
  variables: Record<string, string>
  response: { status: number; content: string; content_type: string }
  dont_save: boolean
  stopped: boolean
}

export type TokenInput = Partial<{
  alias: string
  default_status: number
  default_content: string
  default_content_type: string
  timeout: number
  cors: boolean
  expiry: number
  request_limit: number
  description: string
  redirect: string
  group_id: string
  listen: number
}>

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  listTokens: () => req<{ data: Token[] }>('/tokens').then((r) => r.data ?? []),
  createToken: (body: TokenInput = {}) =>
    req<Token>('/tokens', { method: 'POST', body: JSON.stringify(body) }),
  updateToken: (id: string, body: TokenInput) =>
    req<Token>(`/tokens/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteToken: (id: string) => req<void>(`/tokens/${id}`, { method: 'DELETE' }),

  listRequests: (id: string, page = 1, perPage = 50, q = '') => {
    const params = new URLSearchParams({ page: String(page), per_page: String(perPage) })
    if (q) params.set('q', q)
    return req<RequestPage>(`/tokens/${id}/requests?${params}`)
  },
  deleteRequest: (id: string, rid: string) =>
    req<void>(`/tokens/${id}/requests/${rid}`, { method: 'DELETE' }),
  clearRequests: (id: string, q = '') => {
    const qs = q ? `?q=${encodeURIComponent(q)}` : ''
    return req<{ deleted: number }>(`/tokens/${id}/requests${qs}`, { method: 'DELETE' })
  },

  listActionTypes: () => req<{ data: string[] }>('/action-types').then((r) => r.data ?? []),
  listActions: (id: string) =>
    req<{ data: Action[] }>(`/tokens/${id}/actions`).then((r) => r.data ?? []),
  createAction: (id: string, body: ActionInput) =>
    req<Action>(`/tokens/${id}/actions`, { method: 'POST', body: JSON.stringify(body) }),
  updateAction: (id: string, aid: string, body: ActionInput) =>
    req<Action>(`/tokens/${id}/actions/${aid}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteAction: (id: string, aid: string) =>
    req<void>(`/tokens/${id}/actions/${aid}`, { method: 'DELETE' }),
  testAction: (id: string, body: ActionInput) =>
    req<TestActionResult>(`/tokens/${id}/test-action`, { method: 'POST', body: JSON.stringify(body) }),
  listActionRuns: (id: string, rid: string) =>
    req<{ data: ActionRun[] }>(`/tokens/${id}/requests/${rid}/action-runs`).then((r) => r.data ?? []),
  executeChain: (id: string, rid: string) =>
    req<{ data: ActionRun[] }>(`/tokens/${id}/requests/${rid}/execute`, { method: 'POST' }).then(
      (r) => r.data ?? [],
    ),

  authStatus: () => req<AuthStatus>('/auth/status'),
  login: (email: string, password: string) =>
    req<User>('/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),
  bootstrap: (email: string, password: string) =>
    req<User>('/auth/bootstrap', { method: 'POST', body: JSON.stringify({ email, password }) }),
  logout: () => req<void>('/auth/logout', { method: 'POST' }),

  listAPIKeys: () => req<{ data: APIKey[] }>('/account/api-keys').then((r) => r.data ?? []),
  createAPIKey: (name: string) =>
    req<{ key: string; api_key: APIKey }>('/account/api-keys', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  deleteAPIKey: (id: string) => req<void>(`/account/api-keys/${id}`, { method: 'DELETE' }),

  listUsers: () => req<{ data: User[] }>('/users').then((r) => r.data ?? []),
  createUser: (email: string, password: string, role: string) =>
    req<User>('/users', { method: 'POST', body: JSON.stringify({ email, password, role }) }),
  updateUser: (id: string, body: Partial<{ email: string; password: string; role: string }>) =>
    req<User>(`/users/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteUser: (id: string) => req<void>(`/users/${id}`, { method: 'DELETE' }),

  listSchedules: () => req<{ data: Schedule[] }>('/schedules').then((r) => r.data ?? []),
  createSchedule: (body: ScheduleInput) =>
    req<Schedule>('/schedules', { method: 'POST', body: JSON.stringify(body) }),
  updateSchedule: (id: string, body: ScheduleInput) =>
    req<Schedule>(`/schedules/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteSchedule: (id: string) => req<void>(`/schedules/${id}`, { method: 'DELETE' }),
  runSchedule: (id: string) => req<ScheduleRun>(`/schedules/${id}/run`, { method: 'POST' }),
  listScheduleRuns: (id: string) =>
    req<{ data: ScheduleRun[] }>(`/schedules/${id}/runs`).then((r) => r.data ?? []),

  replay: (id: string, targetURL: string, q = '') =>
    req<{ replayed: number; failed: number }>(`/tokens/${id}/replay`, {
      method: 'POST',
      body: JSON.stringify({ target_url: targetURL, q }),
    }),

  listGroups: () => req<{ data: Group[] }>('/groups').then((r) => r.data ?? []),
  createGroup: (name: string, color = '') =>
    req<Group>('/groups', { method: 'POST', body: JSON.stringify({ name, color }) }),
  updateGroup: (id: string, body: Partial<{ name: string; color: string }>) =>
    req<Group>(`/groups/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteGroup: (id: string) => req<void>(`/groups/${id}`, { method: 'DELETE' }),

  rawURL: (id: string, rid: string) => `/api/v1/tokens/${id}/requests/${rid}/raw`,
  csvURL: (id: string) => `/api/v1/tokens/${id}/requests.csv`,
  streamURL: (id: string) => `/api/v1/tokens/${id}/stream`,
}
