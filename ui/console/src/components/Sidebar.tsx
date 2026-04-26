import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Building2,
  Briefcase,
  LayoutDashboard,
  BarChart2,
  Settings,
  ChevronsLeft,
  ChevronsRight,
  LogOut,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/features/auth/store'
import { LogoLockup } from '@/components/brand/Logo'

const nav: { to: string; label: string; icon: React.ReactNode }[] = [
  { to: '/dashboard', label: 'Dashboard', icon: <LayoutDashboard size={16} /> },
  { to: '/companies', label: 'Companies', icon: <Building2 size={16} /> },
  { to: '/jobs', label: 'Jobs', icon: <Briefcase size={16} /> },
  { to: '/insights', label: 'Insights', icon: <BarChart2 size={16} /> },
  { to: '/settings', label: 'Settings', icon: <Settings size={16} /> },
]

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false)
  const session = useAuthStore((s) => s.session)
  const signOut = useAuthStore((s) => s.signOut)
  const email = session?.user?.email ?? ''
  const initials = email ? email[0].toUpperCase() : '?'

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
      <nav className="flex-1 px-1.5 py-4 space-y-0.5">
        {nav.map(({ to, label, icon }) => (
          <NavLink
            key={to}
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
        ))}
      </nav>

      {/* Footer */}
      <div className={cn('border-t border-white/[0.07]', collapsed ? 'px-1.5 py-3' : 'px-3 py-3')}>
        {collapsed ? (
          <button
            onClick={signOut}
            title={email}
            className="w-full flex items-center justify-center p-2 rounded-lg hover:bg-white/[0.05] transition-colors"
          >
            <span className="flex items-center justify-center w-6 h-6 rounded-full bg-violet-600/30 text-violet-300 text-xs font-semibold shrink-0">
              {initials}
            </span>
          </button>
        ) : (
          <div className="flex items-center gap-2.5">
            <span className="flex items-center justify-center w-7 h-7 rounded-full bg-violet-600/30 text-violet-300 text-xs font-semibold shrink-0">
              {initials}
            </span>
            <span className="flex-1 text-xs text-slate-400 truncate">{email}</span>
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
