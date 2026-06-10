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
        'bg-subtle border border-[var(--border)] rounded-lg px-3.5 py-3 text-left transition-colors',
        interactive && 'cursor-pointer hover:border-violet-500/35 hover:bg-hover',
        active && 'border-violet-500/40 bg-violet-500/[0.05] hover:border-violet-500/50'
      )}
    >
      <div className={cn('text-[22px] leading-tight font-semibold text-primary', valueClassName)}>
        {value}
      </div>
      <div className="text-[10px] uppercase tracking-[0.08em] text-muted mt-1">{label}</div>
    </Tag>
  )
}
