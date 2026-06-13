import { Card } from '@/components/ui/card'
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

// Card's rounded-lg + border are composed into button elements via sharedClassName
// so interactive cards render a native <button> (correct role + type="button" semantics)
// while non-interactive cards render Card (a <div> with the same visual shape).
const CARD_SHAPE = 'rounded-lg border'

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

  const sharedClassName = cn(
    CARD_SHAPE,
    'bg-subtle border-[var(--border)] px-3.5 py-3 text-left transition-colors shadow-none',
    interactive && 'cursor-pointer hover:border-violet-500/35 hover:bg-hover',
    active && 'border-violet-500/40 bg-violet-500/[0.05] hover:border-violet-500/50'
  )

  const inner = (
    <>
      <div className={cn('text-[22px] leading-tight font-semibold text-primary', valueClassName)}>
        {value}
      </div>
      <div className="text-[10px] uppercase tracking-[0.08em] text-muted mt-1">{label}</div>
    </>
  )

  if (interactive) {
    // Native <button> so getByRole('button') and type="button" work correctly in tests and for accessibility.
    return (
      <button type="button" onClick={onClick} title={title} className={sharedClassName}>
        {inner}
      </button>
    )
  }

  return (
    <Card className={sharedClassName} title={title}>
      {inner}
    </Card>
  )
}
