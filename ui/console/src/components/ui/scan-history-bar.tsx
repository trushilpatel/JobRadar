import { cn } from '@/lib/utils'

export type HistoryPoint = 'ok' | 'err' | 'run' | 'idle'

interface ScanHistoryBarProps {
  history: HistoryPoint[]
  className?: string
}

const BAR_CLASS: Record<HistoryPoint, string> = {
  ok:   'bg-emerald-500/70',
  err:  'bg-red-500/80',
  run:  'bg-blue-500 animate-pulse',
  idle: 'bg-white/10',
}

const TITLE: Record<HistoryPoint, string> = {
  ok: 'OK', err: 'Error', run: 'Running', idle: 'Idle',
}

export function ScanHistoryBar({ history, className }: ScanHistoryBarProps) {
  if (!history.length) {
    return <span className="text-[10px] text-slate-600">—</span>
  }
  return (
    <div className={cn('inline-flex gap-0.5', className)}>
      {history.map((p, i) => (
        <span
          key={i}
          title={TITLE[p]}
          className={cn('w-[6px] h-4 rounded-sm', BAR_CLASS[p])}
        />
      ))}
    </div>
  )
}
