import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Building2, Briefcase, LayoutDashboard, Send, BarChart2, ChevronsLeft, ChevronsRight } from 'lucide-react'
import { cn } from '@/lib/utils'

const nav: { to: string; label: string; icon: React.ReactNode }[] = [
  { to: '/dashboard',    label: 'Dashboard',    icon: <LayoutDashboard size={16} /> },
  { to: '/companies',    label: 'Companies',    icon: <Building2 size={16} /> },
  { to: '/jobs',         label: 'Jobs',         icon: <Briefcase size={16} /> },
  { to: '/applications', label: 'Applications', icon: <Send size={16} /> },
  { to: '/insights',     label: 'Insights',     icon: <BarChart2 size={16} /> },
]

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false)

  return (
    <div
      className={cn(
        'shrink-0 bg-[#0d0d14] border-r border-white/[0.07] flex flex-col transition-[width] duration-200 overflow-hidden',
        collapsed ? 'w-[52px]' : 'w-56'
      )}
    >
      {/* Logo + collapse toggle */}
      <div className={cn(
        'flex items-center border-b border-white/[0.07] h-[57px]',
        collapsed ? 'justify-center px-0' : 'justify-between px-5'
      )}>
        {!collapsed && (
          <div>
            <span className="text-white font-semibold tracking-tight">JobRadar</span>
            <span className="ml-2 text-xs text-violet-400 font-medium">beta</span>
          </div>
        )}
        <button
          onClick={() => setCollapsed(v => !v)}
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
      {!collapsed && (
        <div className="px-5 py-4 border-t border-white/[0.07]">
          <p className="text-xs text-slate-500">Single-user mode</p>
        </div>
      )}
    </div>
  )
}
