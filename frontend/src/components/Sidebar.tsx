import { Building2, Briefcase, LayoutDashboard, Send } from 'lucide-react'
import { cn } from '@/lib/utils'

export type Page = 'dashboard' | 'companies' | 'jobs' | 'applications'

interface Props {
  current: Page
  onChange: (page: Page) => void
}

const nav: { key: Page; label: string; icon: React.ReactNode }[] = [
  { key: 'dashboard', label: 'Dashboard', icon: <LayoutDashboard size={16} /> },
  { key: 'companies', label: 'Companies', icon: <Building2 size={16} /> },
  { key: 'jobs', label: 'Jobs', icon: <Briefcase size={16} /> },
  { key: 'applications', label: 'Applications', icon: <Send size={16} /> },
]

export function Sidebar({ current, onChange }: Props) {
  return (
    <div className="w-56 shrink-0 bg-[#0d0d14] border-r border-white/[0.07] flex flex-col">
      <div className="px-5 py-5 border-b border-white/[0.07]">
        <span className="text-white font-semibold tracking-tight">JobRadar</span>
        <span className="ml-2 text-xs text-violet-400 font-medium">beta</span>
      </div>

      <nav className="flex-1 px-3 py-4 space-y-0.5">
        {nav.map(({ key, label, icon }) => (
          <button
            key={key}
            onClick={() => onChange(key)}
            className={cn(
              'w-full flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-sm transition-colors text-left',
              current === key
                ? 'bg-violet-600/20 text-violet-300 font-medium'
                : 'text-slate-400 hover:text-white hover:bg-white/[0.05]'
            )}
          >
            <span className={current === key ? 'text-violet-400' : ''}>{icon}</span>
            {label}
          </button>
        ))}
      </nav>

      <div className="px-5 py-4 border-t border-white/[0.07]">
        <p className="text-xs text-slate-500">Single-user mode</p>
      </div>
    </div>
  )
}
