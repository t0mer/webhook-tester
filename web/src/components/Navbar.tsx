import { BoltIcon, MenuIcon, MoonIcon, PlusIcon, SunIcon } from './icons'
import type { Theme } from '../useTheme'
import type { User } from '../api'

interface Props {
  theme: Theme
  onToggleTheme: () => void
  onNewToken: () => void
  onToggleSidebar: () => void
  onHome: () => void
  currentUser?: User
  onAccount: () => void
  onLogout: () => void
}

export function Navbar({
  theme,
  onToggleTheme,
  onNewToken,
  onToggleSidebar,
  onHome,
  currentUser,
  onAccount,
  onLogout,
}: Props) {
  return (
    <header className="flex items-center gap-3 border-b border-border bg-surface px-4 h-14 shrink-0">
      <button
        className="md:hidden p-2 -ml-2 rounded-lg hover:bg-surface-2 text-muted"
        onClick={onToggleSidebar}
        aria-label="Toggle token list"
      >
        <MenuIcon />
      </button>

      <button
        onClick={onHome}
        className="flex items-center gap-2 font-semibold tracking-tight"
        aria-label="Go to inbox"
      >
        <span className="grid place-items-center w-8 h-8 rounded-lg bg-accent text-accent-fg">
          <BoltIcon width={18} height={18} />
        </span>
        <span className="text-lg">Raptor</span>
        <span className="hidden sm:inline text-xs text-muted font-normal">webhook inspector</span>
      </button>

      <div className="ml-auto flex items-center gap-2">
        <button
          onClick={onNewToken}
          className="inline-flex items-center gap-1.5 rounded-lg bg-accent text-accent-fg px-3 py-1.5 text-sm font-medium hover:opacity-90 transition"
        >
          <PlusIcon width={16} height={16} />
          <span className="hidden sm:inline">New URL</span>
        </button>
        <button
          onClick={onToggleTheme}
          className="p-2 rounded-lg hover:bg-surface-2 text-muted"
          aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
        </button>

        {currentUser && (
          <div className="flex items-center gap-1 border-l border-border pl-2 ml-1">
            <button
              onClick={onAccount}
              className="text-xs text-muted hover:text-text px-2 py-1 rounded-lg hover:bg-surface-2 max-w-[10rem] truncate"
              title={currentUser.email}
            >
              {currentUser.email}
            </button>
            <button
              onClick={onLogout}
              className="text-xs text-muted hover:text-text px-2 py-1 rounded-lg hover:bg-surface-2"
            >
              Sign out
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
