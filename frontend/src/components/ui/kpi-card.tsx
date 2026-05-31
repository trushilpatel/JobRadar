import { cn } from '@/lib/utils/utils'

interface KpiCardProps {
  value: string | number
  label: string
  valueClassName?: string
  clickable?: boolean
  active?: boolean
  onClick?: () => void
  title?: string
}

export function KpiCard({
  value,
  label,
  valueClassName,
  clickable,
  active,
  onClick,
  title,
}: KpiCardProps) {
  const interactive = clickable || !!onClick
  const Tag = interactive ? 'button' : 'div'
  return (
    <Tag
      type={interactive ? 'button' : undefined}
      onClick={onClick}
      title={title}
      className={cn(
        'bg-white/[0.02] border border-white/[0.06] rounded-lg px-3.5 py-3 text-left transition-colors',
        interactive && 'cursor-pointer hover:border-violet-500/35 hover:bg-white/[0.03]',
        active && 'border-red-500/40 bg-red-500/[0.05] hover:border-red-500/50'
      )}
    >
      <div className={cn('text-[22px] leading-tight font-semibold text-white', valueClassName)}>
        {value}
      </div>
      <div className="text-[10px] uppercase tracking-[0.08em] text-slate-500 mt-1">{label}</div>
    </Tag>
  )
}
