import { cn } from '@/lib/utils'

interface StatColProps {
  label: string
  value: number | string
  sub: string
  valueClass: string
  onClick?: () => void
  onKeyDown?: (e: React.KeyboardEvent) => void
}

export function StatCol({ label, value, sub, valueClass, onClick, onKeyDown }: StatColProps) {
  const isInteractive = Boolean(onClick)
  return (
    <div
      role={isInteractive ? 'button' : undefined}
      tabIndex={isInteractive ? 0 : undefined}
      className={cn(
        'flex-1 flex flex-col gap-1 px-5 first:pl-0 last:pr-0',
        isInteractive &&
          'cursor-pointer group focus:outline-none focus-visible:ring-1 focus-visible:ring-white/30 rounded'
      )}
      onClick={onClick}
      onKeyDown={onKeyDown}
    >
      <span className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500">
        {label}
      </span>
      <span
        className={cn(
          'text-[36px] font-medium leading-none tabular-nums',
          valueClass,
          isInteractive && 'group-hover:opacity-80 transition-opacity'
        )}
      >
        {value}
      </span>
      <span
        className={cn(
          'text-[11px] text-slate-500',
          isInteractive && 'group-hover:text-slate-400 transition-colors'
        )}
      >
        {sub}
      </span>
    </div>
  )
}
