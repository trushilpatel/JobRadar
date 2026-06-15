import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Building2,
  Briefcase,
  LayoutDashboard,
  BarChart2,
  ChevronsLeft,
  ChevronsRight,
  LogOut,
  Users,
  ClipboardList,
  Settings,
  FileText,
  Send,
} from 'lucide-react'
import { useAuth, useUser, useClerk } from '@clerk/react'
import { cn } from '@/lib/utils/utils'
import { routes } from '@/config/routes'
import { COPY } from '@/config/copy'
import { LogoLockup } from '@/components/brand/Logo'

const userNav: { to: string; label: string; icon: React.ReactNode }[] = [
  {
    to: routes.dashboard(),
    label: COPY['nav.dashboard'],
    icon: <LayoutDashboard size={16} />,
  },
  { to: routes.jobs(), label: COPY['nav.jobs'], icon: <Briefcase size={16} /> },
  { to: routes.insights(), label: COPY['nav.insights'], icon: <BarChart2 size={16} /> },
]

const adminNav: { to: string; label: string; icon: React.ReactNode }[] = [
  {
    to: routes.adminCompanies(),
    label: COPY['nav.companies'],
    icon: <Building2 size={16} />,
  },
  { to: routes.adminUsers(), label: COPY['nav.users'], icon: <Users size={16} /> },
  { to: routes.resumeLibrary(), label: COPY['nav.resumes'], icon: <FileText size={16} /> },
  { to: routes.applyQueue(), label: COPY['nav.apply-queue'], icon: <Send size={16} /> },
  {
    to: routes.adminAudit(),
    label: COPY['nav.audit-log'],
    icon: <ClipboardList size={16} />,
  },
  { to: routes.adminPipeline(), label: COPY['nav.pipeline'], icon: <Settings size={16} /> },
]

function NavItem({
  to,
  label,
  icon,
  collapsed,
}: {
  to: string
  label: string
  icon: React.ReactNode
  collapsed: boolean
}) {
  return (
    <NavLink
      to={to}
      title={collapsed ? label : undefined}
      className={({ isActive }) =>
        cn(
          'w-full flex items-center rounded-lg text-sm transition-colors',
          collapsed ? 'justify-center p-2.5' : 'gap-2.5 px-3 py-2.5',
          isActive
            ? 'bg-[var(--sidebar-active)] text-[var(--accent-text)] font-medium'
            : 'text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)]'
        )
      }
    >
      {({ isActive }) => (
        <>
          <span className={cn('shrink-0', isActive ? 'text-[var(--accent)]' : '')}>{icon}</span>
          {!collapsed && <span>{label}</span>}
        </>
      )}
    </NavLink>
  )
}

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false)
  const { user } = useUser()
  const { isSignedIn } = useAuth()
  const { signOut } = useClerk()
  const isAdmin = user?.publicMetadata?.role === 'admin'
  const email = isSignedIn ? (user?.primaryEmailAddress?.emailAddress ?? '') : ''
  const initials = email ? email[0]?.toUpperCase() : '?'

  return (
    <div
      className={cn(
        'shrink-0 bg-sidebar border-r border-[var(--sidebar-border)] flex flex-col transition-[width] duration-200 overflow-hidden',
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
            <span className="text-xs text-violet-400 font-medium">beta</span>
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

      {/* Nav links */}
      <nav className="flex-1 px-1.5 py-4 space-y-0.5 overflow-y-auto">
        {userNav.map(({ to, label, icon }) => (
          <NavItem key={to} to={to} label={label} icon={icon} collapsed={collapsed} />
        ))}

        {isAdmin && (
          <>
            {!collapsed && (
              <p className="px-3 pt-4 pb-1 text-[10px] font-semibold uppercase tracking-widest text-slate-600 select-none">
                Admin
              </p>
            )}
            {collapsed && (
              <div role="separator" className="my-2 border-t border-[var(--sidebar-border)]" />
            )}
            {adminNav.map(({ to, label, icon }) => (
              <NavItem key={to} to={to} label={label} icon={icon} collapsed={collapsed} />
            ))}
          </>
        )}
      </nav>

      {/* Footer */}
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
                className={({ isActive }) =>
                  cn(
                    'w-full flex items-center justify-center p-2 rounded-lg transition-colors',
                    isActive ? 'bg-[var(--sidebar-active)]' : 'hover:bg-[var(--sidebar-hover)]'
                  )
                }
              >
                <span className="flex items-center justify-center w-6 h-6 rounded-full bg-violet-600/30 text-violet-300 text-xs font-semibold shrink-0">
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
              className={({ isActive }) =>
                cn(
                  'flex-1 flex items-center gap-2 rounded-lg px-1 py-1 -mx-1 transition-colors min-w-0',
                  isActive
                    ? 'text-[var(--accent-text)]'
                    : 'text-muted hover:text-[var(--sidebar-fg)] hover:bg-[var(--sidebar-hover)]'
                )
              }
            >
              <span className="flex items-center justify-center w-7 h-7 rounded-full bg-violet-600/30 text-violet-300 text-xs font-semibold shrink-0">
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
    </div>
  )
}
