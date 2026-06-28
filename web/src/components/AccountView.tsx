import { useEffect, useState } from 'react'
import { api, type APIKey, type User } from '../api'
import { copyText, relativeTime } from '../lib'
import { CopyIcon, TrashIcon } from './icons'

const field =
  'rounded-lg border border-border bg-surface-2 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-accent/40'

export function AccountView({ currentUser }: { currentUser?: User }) {
  return (
    <div className="flex-1 overflow-y-auto p-4 sm:p-6 max-w-3xl mx-auto w-full space-y-8">
      <div>
        <h1 className="text-xl font-semibold mb-1">Account</h1>
        {currentUser && (
          <p className="text-sm text-muted">
            Signed in as <span className="font-mono">{currentUser.email}</span> ({currentUser.role})
          </p>
        )}
      </div>

      <ApiKeys />
      {currentUser?.role === 'admin' && <Users currentUser={currentUser} />}
    </div>
  )
}

function ApiKeys() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [name, setName] = useState('')
  const [created, setCreated] = useState<string | null>(null)

  const reload = () => api.listAPIKeys().then(setKeys).catch(() => {})
  useEffect(() => {
    void reload()
  }, [])

  async function create() {
    const res = await api.createAPIKey(name.trim())
    setCreated(res.key)
    setName('')
    void reload()
  }
  async function remove(k: APIKey) {
    await api.deleteAPIKey(k.id)
    void reload()
  }

  return (
    <section>
      <h2 className="text-sm uppercase tracking-wide text-muted mb-2">API keys</h2>
      <p className="text-xs text-muted mb-3">
        Use an API key with the <code className="font-mono">Api-Key</code> header for programmatic
        access.
      </p>

      {created && (
        <div className="rounded-lg border border-ok/40 bg-ok/10 p-3 mb-3 text-sm">
          <div className="text-xs text-muted mb-1">Copy your new key now — it won't be shown again:</div>
          <div className="flex items-center gap-2">
            <code className="font-mono text-xs break-all">{created}</code>
            <button
              onClick={() => copyText(created)}
              className="p-1 rounded hover:bg-surface-2 text-muted shrink-0"
              aria-label="Copy key"
            >
              <CopyIcon width={14} height={14} />
            </button>
          </div>
        </div>
      )}

      <div className="flex gap-2 mb-3">
        <input
          className={field}
          placeholder="Key name (e.g. ci)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button onClick={create} className="rounded-lg bg-accent text-accent-fg px-3 py-1.5 text-sm font-medium">
          Create key
        </button>
      </div>

      <ul className="space-y-1">
        {keys.length === 0 && <li className="text-sm text-muted">No API keys yet.</li>}
        {keys.map((k) => (
          <li key={k.id} className="flex items-center gap-3 rounded-lg border border-border px-3 py-2 text-sm">
            <span className="font-medium">{k.name || '(unnamed)'}</span>
            <span className="text-xs text-muted">
              {k.last_used_at ? `last used ${relativeTime(k.last_used_at)}` : 'never used'}
            </span>
            <button
              onClick={() => remove(k)}
              className="ml-auto p-1 rounded hover:bg-err/10 text-err"
              aria-label="Revoke key"
            >
              <TrashIcon width={14} height={14} />
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

function Users({ currentUser }: { currentUser: User }) {
  const [users, setUsers] = useState<User[]>([])
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState('user')
  const [error, setError] = useState<string | null>(null)

  const reload = () => api.listUsers().then(setUsers).catch(() => {})
  useEffect(() => {
    void reload()
  }, [])

  async function add() {
    setError(null)
    try {
      await api.createUser(email.trim(), password, role)
      setEmail('')
      setPassword('')
      void reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed')
    }
  }
  async function changeRole(u: User, newRole: string) {
    await api.updateUser(u.id, { role: newRole })
    void reload()
  }
  async function remove(u: User) {
    await api.deleteUser(u.id)
    void reload()
  }

  return (
    <section>
      <h2 className="text-sm uppercase tracking-wide text-muted mb-2">Users</h2>
      <div className="flex flex-wrap gap-2 mb-3">
        <input className={field} placeholder="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        <input
          className={field}
          type="password"
          placeholder="password (8+ chars)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <select className={field} value={role} onChange={(e) => setRole(e.target.value)}>
          <option value="user">user</option>
          <option value="admin">admin</option>
        </select>
        <button onClick={add} className="rounded-lg bg-accent text-accent-fg px-3 py-1.5 text-sm font-medium">
          Add user
        </button>
      </div>
      {error && <div className="text-sm text-err mb-2">{error}</div>}

      <ul className="space-y-1">
        {users.map((u) => (
          <li key={u.id} className="flex items-center gap-3 rounded-lg border border-border px-3 py-2 text-sm">
            <span className="font-mono">{u.email}</span>
            <select
              className="rounded-md border border-border bg-surface-2 px-2 py-1 text-xs"
              value={u.role}
              onChange={(e) => changeRole(u, e.target.value)}
              disabled={u.id === currentUser.id}
            >
              <option value="user">user</option>
              <option value="admin">admin</option>
            </select>
            {u.id !== currentUser.id && (
              <button
                onClick={() => remove(u)}
                className="ml-auto p-1 rounded hover:bg-err/10 text-err"
                aria-label="Delete user"
              >
                <TrashIcon width={14} height={14} />
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
