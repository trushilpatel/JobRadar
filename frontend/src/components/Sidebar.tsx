import { NavLink } from 'react-router-dom'
import { Building2, Briefcase, LayoutDashboard, Send, BarChart2 } from 'lucide-react'
import { cn } from '@/lib/utils'

const nav: { to: string; label: string; icon: React.ReactNode }[] = [
  { to: '/dashboard',    label: 'Dashboard',    icon: <LayoutDashboard size={16} /> },
  { to: '/companies',    label: 'Companies',    icon: <Building2 size={16} /> },
  { to: '/jobs',         label: 'Jobs',         icon: <Briefcase size={16} /> },
  { to: '/applications', label: 'Applications', icon: <Send size={16} /> },
  { to: '/insights',     label: 'Insights',     icon: <BarChart2 size={16} /> },
]

export function Sidebar() {
  return (
    <div className="w-56 shrink-0 bg-[#0d0d14] border-r border-white/[0.07] flex flex-col">
      <div className="px-5 py-5 border-b border-white/[0.07]">
        <span className="text-white font-semibold tracking-tight">JobRadar</span>
        <span className="ml-2 text-xs text-violet-400 font-medium">beta</span>
      </div>

      <nav className="flex-1 px-3 py-4 space-y-0.5">
        {nav.map(({ to, label, icon }) => (
          <NavLink
            key={to}
            to={to}
            className={({ isActive }) =>
              cn(
                'w-full flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-sm transition-colors text-left',
                isActive
                  ? 'bg-violet-600/20 text-violet-300 font-medium'
                  : 'text-slate-400 hover:text-white hover:bg-white/[0.05]'
              )
            }
          >
            {({ isActive }) => (
              <>
                <span className={isActive ? 'text-violet-400' : ''}>{icon}</span>
                {label}
              </>
            )}
          </NavLink>
        ))}
      </nav>

      <div className="px-5 py-4 border-t border-white/[0.07]">
        <p className="text-xs text-slate-500">Single-user mode</p>
      </div>
    </div>
  )
}
