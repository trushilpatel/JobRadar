import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Building2,
  Briefcase,
  ChevronsLeft,
  ChevronsRight,
  LogOut,
  Users,
  Settings,
  FileText,
  Send,
} from 'lucide-react'
import { useAuth, useUser, useClerk } from '@clerk/react'
import { cn } from '@/lib/utils/utils'
import { routes } from '@/config/routes'
import { COPY } from '@/config/copy'
import { LogoLockup } from '@/components/brand/Logo'

const APP_VERSION = 'v1.0.0'

interface NavLinkSpec {
  to: string
  label: string
  icon: React.ReactNode
}

// Résumés + Apply queue are frontend-only pages (no backend endpoints), so
// they are user-facing for every authenticated user — not admin-gated.
const userNav: NavLinkSpec[] = [
  { to: routes.jobs(), label: COPY['nav.jobs'], icon: <Briefcase size={16} /> },
  { to: routes.applyQueue(), label: COPY['nav.apply-queue'], icon: <Send size={16} /> },
  { to: routes.resumeLibrary(), label: COPY['nav.resumes'], icon: <FileText size={16} /> },
]

const adminNav: NavLinkSpec[] = [
  {
    to: routes.adminCompanies(),
    label: COPY['nav.companies'],
    icon: <Building2 size={16} />,
  },
  { to: routes.adminUsers(), label: COPY['nav.users'], icon: <Users size={16} /> },
  { to: routes.adminPipeline(), label: COPY['nav.pipeline'], icon: <Settings size={16} /> },
]

function NavItem({
  to,
  label,
  icon,
  collapsed,
  onNavigate,
}: {
  to: string
  label: string
  icon: React.ReactNode
  collapsed: boolean
  onNavigate?: () => void
}) {
  return (
    <NavLink
      to={to}
      title={collapsed ? label : undefined}
      onClick={onNavigate}
      className={({ isActive }) =>
        cn(
          'w-full flex items-center rounded-lg text-sm transition-colors',
          collapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2.5',
          isActive
            ? 'bg-[var(--sidebar-active)] text-[var(--accent-text)] font-semibold'
            : 'text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)]'
        )
      }
      style={({ isActive }) =>
        isActive
          ? { boxShadow: 'inset 0 0 0 1px color-mix(in srgb, var(--violet) 28%, transparent)' }
          : undefined
      }
    >
      {({ isActive }) => (
        <>
          <span
            className={cn('shrink-0 transition-colors', isActive ? 'text-[var(--accent)]' : '')}
          >
            {icon}
          </span>
          {!collapsed && <span>{label}</span>}
        </>
      )}
    </NavLink>
  )
}

/**
 * SidebarNav — the single source of truth for the navigation list, account
 * footer, and version line. Rendered identically by the desktop rail and the
 * mobile drawer (Sheet) so the two surfaces never drift.
 *
 * `collapsed` only ever true on the desktop rail; the mobile drawer always
 * renders expanded. `onNavigate` lets the drawer close itself on link click.
 */
function SidebarNav({ collapsed, onNavigate }: { collapsed: boolean; onNavigate?: () => void }) {
  const { user } = useUser()
  const { isSignedIn } = useAuth()
  const { signOut } = useClerk()
  const isAdmin = user?.publicMetadata?.role === 'admin'
  const email = isSignedIn ? (user?.primaryEmailAddress?.emailAddress ?? '') : ''
  const initials = email ? email[0]?.toUpperCase() : '?'

  return (
    <>
      {/* Nav links */}
      <nav className="flex-1 px-1.5 py-4 space-y-0.5 overflow-y-auto">
        {userNav.map(({ to, label, icon }) => (
          <NavItem
            key={to}
            to={to}
            label={label}
            icon={icon}
            collapsed={collapsed}
            onNavigate={onNavigate}
          />
        ))}

        {isAdmin && (
          <>
            {!collapsed && (
              <p className="px-3 pt-4 pb-1 text-[10px] font-semibold uppercase tracking-widest text-placeholder select-none">
                Admin
              </p>
            )}
            {collapsed && (
              <div role="separator" className="my-2 border-t border-[var(--sidebar-border)]" />
            )}
            {adminNav.map(({ to, label, icon }) => (
              <NavItem
                key={to}
                to={to}
                label={label}
                icon={icon}
                collapsed={collapsed}
                onNavigate={onNavigate}
              />
            ))}
          </>
        )}
      </nav>

      {/* Account footer */}
      <div
        className={cn(
          'border-t border-[var(--sidebar-border)]',
          collapsed ? 'px-1.5 py-3' : 'px-3 py-3'
        )}
      >
        {collapsed ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <NavLink
                to="/account"
                onClick={onNavigate}
                className={({ isActive }) =>
                  cn(
                    'w-full flex items-center justify-center p-2 rounded-lg transition-colors',
                    isActive ? 'bg-[var(--sidebar-active)]' : 'hover:bg-[var(--sidebar-hover)]'
                  )
                }
              >
                <span
                  className="flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold shrink-0"
                  style={{
                    background: 'color-mix(in srgb, var(--violet) 18%, transparent)',
                    color: 'var(--violet-bright)',
                  }}
                >
                  {initials}
                </span>
              </NavLink>
            </TooltipTrigger>
            <TooltipContent side="right">{email}</TooltipContent>
          </Tooltip>
        ) : (
          <div className="flex items-center gap-2.5">
            <NavLink
              to="/account"
              title="Account"
              onClick={onNavigate}
              className={({ isActive }) =>
                cn(
                  'flex-1 flex items-center gap-2 rounded-lg px-1 py-1 -mx-1 transition-colors min-w-0',
                  isActive
                    ? 'text-[var(--accent-text)]'
                    : 'text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)]'
                )
              }
            >
              <span
                className="flex items-center justify-center w-7 h-7 rounded-full text-xs font-semibold shrink-0"
                style={{
                  background: 'color-mix(in srgb, var(--violet) 18%, transparent)',
                  color: 'var(--violet-bright)',
                }}
              >
                {initials}
              </span>
              <span className="text-xs truncate">{email}</span>
            </NavLink>
            <button
              onClick={() => void signOut()}
              title="Sign out"
              className="p-1 text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)] rounded-md transition-colors shrink-0"
            >
              <LogOut size={13} />
            </button>
          </div>
        )}
      </div>

      {/* Version line */}
      <div
        className={cn(
          'border-t border-[var(--sidebar-border)] px-3 py-2 text-[10px] text-muted select-none',
          collapsed ? 'text-center' : ''
        )}
      >
        {collapsed ? APP_VERSION : `JobRadar · ${APP_VERSION}`}
      </div>
    </>
  )
}

/**
 * Sidebar — the desktop collapsible rail. Hidden on small screens; the mobile
 * drawer in ProtectedLayout renders the same SidebarNav via a Sheet.
 */
export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false)

  return (
    <div
      className={cn(
        'hidden md:flex shrink-0 flex-col overflow-hidden transition-[width] duration-200',
        'bg-sidebar backdrop-blur-xl border-r border-[var(--sidebar-border)]',
        collapsed ? 'w-[52px]' : 'w-56'
      )}
    >
      {/* Logo + collapse toggle */}
      <div
        className={cn(
          'flex items-center border-b border-[var(--sidebar-border)] h-[57px]',
          collapsed ? 'justify-center px-0' : 'justify-between px-5'
        )}
      >
        {!collapsed && (
          <div className="flex items-center gap-2">
            <LogoLockup className="text-sidebar-fg" />
            <span className="text-xs font-medium" style={{ color: 'var(--violet-bright)' }}>
              beta
            </span>
          </div>
        )}
        <button
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          className="p-1.5 text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)] rounded-md transition-colors shrink-0"
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <ChevronsRight size={14} /> : <ChevronsLeft size={14} />}
        </button>
      </div>

      <SidebarNav collapsed={collapsed} />
    </div>
  )
}

/**
 * MobileSidebarNav — the drawer body. Always expanded; closes on navigation.
 */
export function MobileSidebarNav({ onNavigate }: { onNavigate: () => void }) {
  return (
    <div className="flex h-full flex-col">
      <SidebarNav collapsed={false} onNavigate={onNavigate} />
    </div>
  )
}
