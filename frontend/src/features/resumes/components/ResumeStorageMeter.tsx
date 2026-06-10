import { cn } from '@/lib/utils/utils'
import { RESUME_LIMIT_PER_USER } from '../types'

interface Props {
  count: number
  className?: string
}

export function ResumeStorageMeter({ count, className }: Props) {
  const pct = Math.min(100, Math.round((count / RESUME_LIMIT_PER_USER) * 100))
  const near = pct >= 80
  const full = count >= RESUME_LIMIT_PER_USER

  return (
    <div className={cn('flex items-center gap-3', className)}>
      <div className="flex-1 max-w-xs">
        <div className="h-1.5 w-full rounded-full bg-subtle overflow-hidden">
          <div
            className={cn(
              'h-full transition-all',
              full ? 'bg-red-500' : near ? 'bg-amber-400' : 'bg-violet-500'
            )}
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>
      <div className="text-xs tabular-nums text-muted">
        <span className={cn(full ? 'text-red-300' : near ? 'text-amber-300' : 'text-primary')}>
          {count}
        </span>
        <span className="text-muted"> / {RESUME_LIMIT_PER_USER}</span>
      </div>
    </div>
  )
}
