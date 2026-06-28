import { useState } from 'react'
import { api } from '../api'
import { BoltIcon } from './icons'

interface Props {
  mode: 'login' | 'bootstrap'
  onAuthed: () => void
}

const field =
  'w-full rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent/40'

// Login renders the sign-in form, or the first-run "create admin" form when the
// instance has not been bootstrapped yet.
export function Login({ mode, onAuthed }: Props) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const bootstrap = mode === 'bootstrap'

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (bootstrap) await api.bootstrap(email, password)
      else await api.login(email, password)
      onAuthed()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-screen grid place-items-center bg-bg p-4">
      <form
        onSubmit={submit}
        className="w-full max-w-sm rounded-xl border border-border bg-surface shadow-xl p-6 space-y-4"
      >
        <div className="flex items-center gap-2 justify-center">
          <span className="grid place-items-center w-9 h-9 rounded-lg bg-accent text-accent-fg">
            <BoltIcon width={20} height={20} />
          </span>
          <span className="text-xl font-semibold tracking-tight">Raptor</span>
        </div>
        <h1 className="text-center text-sm text-muted">
          {bootstrap ? 'Create the first admin account' : 'Sign in to continue'}
        </h1>

        <div>
          <label className="text-xs uppercase tracking-wide text-muted">Email</label>
          <input
            className={field}
            type="email"
            autoComplete="username"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoFocus
            required
          />
        </div>
        <div>
          <label className="text-xs uppercase tracking-wide text-muted">Password</label>
          <input
            className={field}
            type="password"
            autoComplete={bootstrap ? 'new-password' : 'current-password'}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          {bootstrap && <p className="text-xs text-muted mt-1">At least 8 characters.</p>}
        </div>

        {error && <div className="text-sm text-err">{error}</div>}

        <button
          type="submit"
          disabled={busy}
          className="w-full rounded-lg bg-accent text-accent-fg px-4 py-2 text-sm font-medium disabled:opacity-50"
        >
          {busy ? 'Please wait…' : bootstrap ? 'Create admin' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
