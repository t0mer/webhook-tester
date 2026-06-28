import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api,
  type AuthStatus,
  type CapturedRequest,
  type Group,
  type Token,
  type TokenInput,
  type User,
} from './api'
import { copyText } from './lib'
import { useTheme } from './useTheme'
import { Navbar } from './components/Navbar'
import { TokenList } from './components/TokenList'
import { RequestList } from './components/RequestList'
import { RequestDetail } from './components/RequestDetail'
import { SettingsDialog } from './components/SettingsDialog'
import { SearchBar } from './components/SearchBar'
import { ControlPanel } from './components/ControlPanel'
import { ActionsEditor } from './components/ActionsEditor'
import { SchedulesView } from './components/SchedulesView'
import { ReplayDialog } from './components/ReplayDialog'
import { AccountView } from './components/AccountView'
import { Login } from './components/Login'
import { CopyIcon, SettingsIcon, TrashIcon } from './components/icons'

const ACTIVE_KEY = 'raptor-active'
const GROUP_COLORS = ['#4f46e5', '#16a34a', '#d97706', '#dc2626', '#0891b2', '#7c3aed', '#db2777']

type View = 'inbox' | 'panel' | 'schedules' | 'account'

function initialState(): { view: View; active: string | null } {
  const hash = window.location.hash.replace(/^#\/?/, '')
  if (hash === 'panel') return { view: 'panel', active: localStorage.getItem(ACTIVE_KEY) }
  if (hash === 'schedules') return { view: 'schedules', active: localStorage.getItem(ACTIVE_KEY) }
  if (hash === 'account') return { view: 'account', active: localStorage.getItem(ACTIVE_KEY) }
  if (hash) return { view: 'inbox', active: hash }
  return { view: 'inbox', active: localStorage.getItem(ACTIVE_KEY) }
}

// App gates the workspace behind authentication: it loads the auth status and
// renders the first-run bootstrap or login form when required.
export default function App() {
  const [status, setStatus] = useState<AuthStatus | null>(null)

  const load = useCallback(() => {
    api
      .authStatus()
      .then(setStatus)
      .catch(() => setStatus({ bootstrapped: true, require_auth: false, authenticated: false }))
  }, [])
  useEffect(() => {
    load()
  }, [load])

  if (!status) return <div className="min-h-screen bg-bg" />

  if (status.require_auth && !status.bootstrapped) {
    return <Login mode="bootstrap" onAuthed={load} />
  }
  if (status.require_auth && !status.authenticated) {
    return <Login mode="login" onAuthed={load} />
  }
  return <Workspace currentUser={status.user} onLogout={load} />
}

function Workspace({ currentUser, onLogout }: { currentUser?: User; onLogout: () => void }) {
  const { theme, toggle } = useTheme()
  const init = initialState()
  const [tokens, setTokens] = useState<Token[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [activeId, setActiveId] = useState<string | null>(init.active)
  const [view, setView] = useState<View>(init.view)
  const [requests, setRequests] = useState<CapturedRequest[]>([])
  const [selected, setSelected] = useState<CapturedRequest | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [showActions, setShowActions] = useState(false)
  const [showReplay, setShowReplay] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const queryRef = useRef('')

  const activeToken = tokens.find((t) => t.uuid === activeId) ?? null

  const loadTokens = useCallback(async () => {
    try {
      const list = await api.listTokens()
      setTokens(list)
      setActiveId((cur) => cur ?? list[0]?.uuid ?? null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load')
    }
  }, [])

  const loadGroups = useCallback(async () => {
    try {
      setGroups(await api.listGroups())
    } catch {
      /* groups are optional */
    }
  }, [])

  useEffect(() => {
    void loadTokens()
    void loadGroups()
  }, [loadTokens, loadGroups])

  const loadRequests = useCallback(async (id: string, q: string) => {
    try {
      const page = await api.listRequests(id, 1, 100, q)
      setRequests(page.data ?? [])
    } catch {
      /* transient */
    }
  }, [])

  // Reload when the search query changes for the active token.
  useEffect(() => {
    queryRef.current = query
    if (activeId) void loadRequests(activeId, query)
  }, [query, activeId, loadRequests])

  // Live stream + polling fallback for the active token.
  useEffect(() => {
    if (!activeId) return
    localStorage.setItem(ACTIVE_KEY, activeId)
    if (view === 'inbox') window.location.hash = `/${activeId}`
    setSelected(null)
    void loadRequests(activeId, queryRef.current)

    const es = new EventSource(api.streamURL(activeId))
    es.addEventListener('request', (e) => {
      const r = JSON.parse((e as MessageEvent).data) as CapturedRequest
      // With an active filter, refetch so results stay consistent with the query.
      if (queryRef.current) {
        void loadRequests(activeId, queryRef.current)
      } else {
        setRequests((prev) => (prev.some((x) => x.uuid === r.uuid) ? prev : [r, ...prev]))
      }
      setTokens((prev) =>
        prev.map((t) => (t.uuid === activeId ? { ...t, latest_request_at: r.created_at } : t)),
      )
    })

    const poll = setInterval(() => void loadRequests(activeId, queryRef.current), 60_000)
    return () => {
      es.close()
      clearInterval(poll)
    }
  }, [activeId, view, loadRequests])

  async function handleCreate() {
    try {
      const tok = await api.createToken({})
      setTokens((prev) => [tok, ...prev])
      setActiveId(tok.uuid)
      setView('inbox')
      setSidebarOpen(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'create failed')
    }
  }

  function handleSelectToken(id: string) {
    setActiveId(id)
    setView('inbox')
    setSidebarOpen(false)
  }

  function goHome() {
    setView('inbox')
    if (activeId) window.location.hash = `/${activeId}`
  }

  function openPanel() {
    setView('panel')
    window.location.hash = '/panel'
    setSidebarOpen(false)
  }

  function openSchedules() {
    setView('schedules')
    window.location.hash = '/schedules'
    setSidebarOpen(false)
  }

  function openAccount() {
    setView('account')
    window.location.hash = '/account'
    setSidebarOpen(false)
  }

  async function handleLogout() {
    try {
      await api.logout()
    } finally {
      onLogout()
    }
  }

  async function handleSaveSettings(body: TokenInput) {
    if (!activeToken) return
    const updated = await api.updateToken(activeToken.uuid, body)
    setTokens((prev) => prev.map((t) => (t.uuid === updated.uuid ? updated : t)))
  }

  async function handleDeleteToken(id: string) {
    await api.deleteToken(id)
    setTokens((prev) => prev.filter((t) => t.uuid !== id))
    if (id === activeId) {
      setShowSettings(false)
      setActiveId(null)
      setRequests([])
    }
  }

  async function handleAssignGroup(tokenId: string, groupId: string) {
    const updated = await api.updateToken(tokenId, { group_id: groupId })
    setTokens((prev) => prev.map((t) => (t.uuid === tokenId ? updated : t)))
  }

  async function handleCreateGroup(name: string) {
    const color = GROUP_COLORS[groups.length % GROUP_COLORS.length]
    const g = await api.createGroup(name, color)
    setGroups((prev) => [g, ...prev])
  }

  async function handleDeleteGroup(id: string) {
    await api.deleteGroup(id)
    setGroups((prev) => prev.filter((g) => g.id !== id))
    setTokens((prev) => prev.map((t) => (t.group_id === id ? { ...t, group_id: '' } : t)))
  }

  async function handleDeleteRequest(rid: string) {
    if (!activeToken) return
    await api.deleteRequest(activeToken.uuid, rid)
    setRequests((prev) => prev.filter((r) => r.uuid !== rid))
    setSelected((s) => (s?.uuid === rid ? null : s))
  }

  async function handleClear() {
    if (!activeToken) return
    await api.clearRequests(activeToken.uuid, queryRef.current)
    void loadRequests(activeToken.uuid, queryRef.current)
    setSelected(null)
  }

  async function copyURL() {
    if (!activeToken) return
    await copyText(activeToken.url)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="h-screen flex flex-col">
      <Navbar
        theme={theme}
        onToggleTheme={toggle}
        onNewToken={handleCreate}
        onToggleSidebar={() => setSidebarOpen((o) => !o)}
        onHome={goHome}
        currentUser={currentUser}
        onAccount={openAccount}
        onLogout={handleLogout}
      />

      {error && (
        <div className="bg-err/10 text-err text-sm px-4 py-2 border-b border-err/20">{error}</div>
      )}

      <div className="flex-1 flex overflow-hidden relative">
        {view === 'inbox' && (
          <>
            <aside
              className={`absolute md:static z-30 h-full w-72 shrink-0 bg-surface border-r border-border flex flex-col transition-transform md:translate-x-0 ${
                sidebarOpen ? 'translate-x-0 shadow-xl' : '-translate-x-full'
              }`}
            >
              <TokenList
                tokens={tokens}
                groups={groups}
                activeId={activeId}
                onSelect={handleSelectToken}
                onOpenPanel={openPanel}
                onOpenSchedules={openSchedules}
              />
            </aside>
            {sidebarOpen && (
              <div
                className="absolute inset-0 z-20 bg-black/40 md:hidden"
                onClick={() => setSidebarOpen(false)}
              />
            )}
          </>
        )}

        {view === 'panel' ? (
          <ControlPanel
            tokens={tokens}
            groups={groups}
            onOpenToken={handleSelectToken}
            onDeleteToken={handleDeleteToken}
            onAssignGroup={handleAssignGroup}
            onCreateGroup={handleCreateGroup}
            onDeleteGroup={handleDeleteGroup}
          />
        ) : view === 'schedules' ? (
          <SchedulesView />
        ) : view === 'account' ? (
          <AccountView currentUser={currentUser} />
        ) : activeToken ? (
          <main className="flex-1 flex flex-col min-w-0">
            <TokenBar
              token={activeToken}
              copied={copied}
              onCopy={copyURL}
              onSettings={() => setShowSettings(true)}
              onActions={() => setShowActions(true)}
              onReplay={() => setShowReplay(true)}
              onClear={handleClear}
            />
            <div className="flex-1 flex min-h-0">
              <div
                className={`w-full md:w-80 lg:w-96 shrink-0 border-r border-border min-h-0 flex flex-col ${
                  selected ? 'hidden md:flex' : 'flex'
                }`}
              >
                <SearchBar onSearch={setQuery} />
                <div className="flex-1 min-h-0">
                  <RequestList
                    requests={requests}
                    activeId={selected?.uuid ?? null}
                    onSelect={setSelected}
                  />
                </div>
              </div>
              <div className={`flex-1 min-w-0 ${selected ? 'block' : 'hidden md:block'}`}>
                {selected ? (
                  <div className="h-full flex flex-col">
                    <button
                      className="md:hidden text-sm text-accent px-4 py-2 text-left border-b border-border"
                      onClick={() => setSelected(null)}
                    >
                      ← Back to requests
                    </button>
                    <div className="flex-1 min-h-0">
                      <RequestDetail
                        tokenId={activeToken.uuid}
                        request={selected}
                        onDelete={handleDeleteRequest}
                      />
                    </div>
                  </div>
                ) : (
                  <div className="hidden md:grid place-items-center h-full text-sm text-muted">
                    Select a request to inspect it
                  </div>
                )}
              </div>
            </div>
          </main>
        ) : (
          <main className="flex-1 grid place-items-center p-8 text-center">
            <div className="max-w-sm">
              <h2 className="text-lg font-semibold mb-2">No URL selected</h2>
              <p className="text-sm text-muted mb-4">
                Create a unique URL and every request sent to it is captured and shown here in real
                time.
              </p>
              <button
                onClick={handleCreate}
                className="rounded-lg bg-accent text-accent-fg px-4 py-2 text-sm font-medium"
              >
                Create your first URL
              </button>
            </div>
          </main>
        )}
      </div>

      {showSettings && activeToken && (
        <SettingsDialog
          token={activeToken}
          groups={groups}
          onClose={() => setShowSettings(false)}
          onSave={handleSaveSettings}
          onDelete={() => handleDeleteToken(activeToken.uuid)}
        />
      )}

      {showActions && activeToken && (
        <ActionsEditor tokenId={activeToken.uuid} onClose={() => setShowActions(false)} />
      )}

      {showReplay && activeToken && (
        <ReplayDialog tokenId={activeToken.uuid} query={query} onClose={() => setShowReplay(false)} />
      )}
    </div>
  )
}

function TokenBar({
  token,
  copied,
  onCopy,
  onSettings,
  onActions,
  onReplay,
  onClear,
}: {
  token: Token
  copied: boolean
  onCopy: () => void
  onSettings: () => void
  onActions: () => void
  onReplay: () => void
  onClear: () => void
}) {
  return (
    <div className="flex items-center gap-2 px-4 h-12 border-b border-border bg-surface shrink-0">
      <code className="font-mono text-xs sm:text-sm truncate text-text">{token.url}</code>
      <button
        onClick={onCopy}
        className="p-1.5 rounded-lg hover:bg-surface-2 text-muted shrink-0"
        aria-label="Copy URL"
      >
        <CopyIcon width={16} height={16} />
      </button>
      {copied && <span className="text-xs text-ok shrink-0">copied</span>}
      <div className="ml-auto flex items-center gap-1 shrink-0">
        <button
          onClick={onActions}
          className={`text-xs px-2 py-1 rounded-lg hover:bg-surface-2 ${
            token.actions ? 'text-accent' : 'text-muted'
          }`}
        >
          Actions
        </button>
        <button
          onClick={onReplay}
          className="text-xs px-2 py-1 rounded-lg hover:bg-surface-2 text-muted"
        >
          Replay
        </button>
        <a
          href={api.csvURL(token.uuid)}
          className="text-xs px-2 py-1 rounded-lg hover:bg-surface-2 text-muted"
        >
          CSV
        </a>
        <button
          onClick={onClear}
          className="p-1.5 rounded-lg hover:bg-surface-2 text-muted"
          aria-label="Clear matching requests"
        >
          <TrashIcon width={16} height={16} />
        </button>
        <button
          onClick={onSettings}
          className="p-1.5 rounded-lg hover:bg-surface-2 text-muted"
          aria-label="URL settings"
        >
          <SettingsIcon width={16} height={16} />
        </button>
      </div>
    </div>
  )
}
