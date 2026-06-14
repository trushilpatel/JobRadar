import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils/utils'

export type StatusKind = 'ok' | 'error' | 'running' | 'queued' | 'pending'

interface StatusPillProps {
  status: StatusKind
  label?: string
  className?: string
}

const CONFIG: Record<StatusKind, { text: string; cls: string; dot: string; pulse?: boolean }> = {
  ok: {
    text: 'OK',
    cls: 'text-emerald-400 bg-emerald-500/10 border border-emerald-500/20',
    dot: 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.5)]',
  },
  error: {
    text: 'Error',
    cls: 'text-red-400 bg-red-500/10 border border-red-500/20',
    dot: 'bg-red-500 shadow-[0_0_6px_rgba(239,68,68,0.5)]',
  },
  running: {
    text: 'Running',
    cls: 'text-blue-400 bg-blue-500/10 border border-blue-500/20',
    dot: 'bg-blue-500',
    pulse: true,
  },
  queued: {
    text: 'Queued',
    cls: 'text-amber-400 bg-amber-500/10 border border-amber-500/20',
    dot: 'bg-amber-500',
  },
  pending: {
    text: 'Not scanned',
    cls: 'text-muted bg-subtle border border-[var(--border)]',
    dot: 'bg-slate-500',
  },
}

export function StatusPill({ status, label, className }: StatusPillProps) {
  const c = CONFIG[status]
  return (
    <Badge className={cn('text-[11px] px-2 py-0.5 gap-1.5', c.cls, className)}>
      <span
        className={cn(
          'w-[7px] h-[7px] rounded-full inline-block',
          c.dot,
          c.pulse && 'animate-pulse'
        )}
      />
      {label ?? c.text}
    </Badge>
  )
}
