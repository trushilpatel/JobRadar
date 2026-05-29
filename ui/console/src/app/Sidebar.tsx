import { useState } from 'react'
import { NavLink } from 'react-router-dom'
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
import { cn } from '@/lib/utils/utils'
import { routes } from '@/lib/routes'
import { useAuthStore } from '@/features/auth/store'
import { LogoLockup } from '@/components/brand/Logo'

const userNav: { to: string; label: string; icon: React.ReactNode }[] = [
  { to: routes.dashboard(), label: 'Dashboard', icon: <LayoutDashboard size={16} /> },
  { to: routes.jobs(), label: 'Jobs', icon: <Briefcase size={16} /> },
]

const adminNav: { to: string; label: string; icon: React.ReactNode }[] = [
  { to: routes.adminCompanies(), label: 'Companies', icon: <Building2 size={16} /> },
  { to: routes.adminUsers(), label: 'Users', icon: <Users size={16} /> },
  { to: routes.resumeLibrary(), label: 'Resumes', icon: <FileText size={16} /> },
  { to: routes.applyQueue(), label: 'Apply Queue', icon: <Send size={16} /> },
  { to: routes.adminAudit(), label: 'Audit Log', icon: <ClipboardList size={16} /> },
  { to: routes.insights(), label: 'Insights', icon: <BarChart2 size={16} /> },
  { to: routes.settings(), label: 'Settings', icon: <Settings size={16} /> },
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
            ? 'bg-violet-600/20 text-violet-300 font-medium'
            : 'text-slate-400 hover:text-white hover:bg-white/[0.05]'
        )
      }
    >
      {({ isActive }) => (
        <>
          <span className={cn('shrink-0', isActive ? 'text-violet-400' : '')}>{icon}</span>
          {!collapsed && <span>{label}</span>}
        </>
      )}
    </NavLink>
  )
}

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false)
  const session = useAuthStore((s) => s.session)
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const signOut = useAuthStore((s) => s.signOut)
  const email = session?.user?.email ?? ''
  const initials = email ? email[0]?.toUpperCase() : '?'

  return (
    <div
      className={cn(
        'shrink-0 bg-[#0d0d14] border-r border-white/[0.07] flex flex-col transition-[width] duration-200 overflow-hidden',
        collapsed ? 'w-[52px]' : 'w-56'
      )}
    >
      {/* Logo + collapse toggle */}
      <div
        className={cn(
          'flex items-center border-b border-white/[0.07] h-[57px]',
          collapsed ? 'justify-center px-0' : 'justify-between px-5'
        )}
      >
        {!collapsed && (
          <div className="flex items-center gap-2">
            <LogoLockup className="text-white" />
            <span className="text-xs text-violet-400 font-medium">beta</span>
          </div>
        )}
        <button
          onClick={() => setCollapsed((v) => !v)}
          className="p-1.5 text-slate-500 hover:text-white hover:bg-white/[0.06] rounded-md transition-colors shrink-0"
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
            {collapsed && <div className="my-2 border-t border-white/[0.07]" />}
            {adminNav.map(({ to, label, icon }) => (
              <NavItem key={to} to={to} label={label} icon={icon} collapsed={collapsed} />
            ))}
          </>
        )}
      </nav>

      {/* Footer */}
      <div className={cn('border-t border-white/[0.07]', collapsed ? 'px-1.5 py-3' : 'px-3 py-3')}>
        {collapsed ? (
          <NavLink
            to="/account"
            title={email}
            className={({ isActive }) =>
              cn(
                'w-full flex items-center justify-center p-2 rounded-lg transition-colors',
                isActive ? 'bg-violet-600/20' : 'hover:bg-white/[0.05]'
              )
            }
          >
            <span className="flex items-center justify-center w-6 h-6 rounded-full bg-violet-600/30 text-violet-300 text-xs font-semibold shrink-0">
              {initials}
            </span>
          </NavLink>
        ) : (
          <div className="flex items-center gap-2.5">
            <NavLink
              to="/account"
              title="Account"
              className={({ isActive }) =>
                cn(
                  'flex-1 flex items-center gap-2 rounded-lg px-1 py-1 -mx-1 transition-colors min-w-0',
                  isActive
                    ? 'text-violet-300'
                    : 'text-slate-400 hover:text-white hover:bg-white/[0.05]'
                )
              }
            >
              <span className="flex items-center justify-center w-7 h-7 rounded-full bg-violet-600/30 text-violet-300 text-xs font-semibold shrink-0">
                {initials}
              </span>
              <span className="text-xs truncate">{email}</span>
            </NavLink>
            <button
              onClick={signOut}
              title="Sign out"
              className="p-1 text-slate-500 hover:text-white hover:bg-white/[0.06] rounded-md transition-colors shrink-0"
            >
              <LogOut size={13} />
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
