import * as Tooltip from '@radix-ui/react-tooltip'
import { cn } from '@/lib/utils/utils'
import type { RetryTierCounts } from '@/types'
import { useRetryTier } from '../hooks/useRetryTier'

const TIERS: Array<{ key: keyof RetryTierCounts; label: string }> = [
  { key: '1m', label: '1m' },
  { key: '10m', label: '10m' },
  { key: '1h', label: '1h' },
  { key: 'dlq', label: 'DLQ' },
]

interface RetryStageProps {
  label: string
  worker: 'filter' | 'enricher'
  counts: RetryTierCounts
}

export function RetryStage({ label, worker, counts }: RetryStageProps) {
  const { mutate: retry, isPending } = useRetryTier()
  const total = counts['1m'] + counts['10m'] + counts['1h'] + counts.dlq

  return (
    <Tooltip.Provider delayDuration={200}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <div className="flex flex-col items-center min-w-[70px] cursor-default select-none">
            <span
              className={cn(
                'text-[18px] font-semibold leading-tight tabular-nums',
                total === 0 ? 'text-white/25' : 'text-amber-400'
              )}
            >
              {total}
            </span>
            <span
              className={cn(
                'text-[10px] uppercase tracking-[0.08em] mt-0.5 text-center',
                total === 0 ? 'text-slate-700' : 'text-slate-500'
              )}
            >
              {label.split(' ').map((word, i) => (
                <span key={i} className="block">
                  {word}
                </span>
              ))}
            </span>
          </div>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            className="bg-slate-800 border border-slate-700 rounded-lg p-3 shadow-xl z-50 min-w-[170px]"
            sideOffset={8}
          >
            <div className="space-y-1.5">
              {TIERS.map(({ key, label: tierLabel }) => (
                <div key={key} className="flex items-center gap-2">
                  <span className="text-slate-400 text-xs font-mono w-8 shrink-0">{tierLabel}</span>
                  <span className="text-white text-xs tabular-nums flex-1 text-right">
                    {counts[key]}
                  </span>
                  <button
                    disabled={counts[key] === 0 || isPending}
                    onClick={() => retry({ worker, tier: key })}
                    className={cn(
                      'text-[10px] px-1.5 py-0.5 rounded border transition-colors shrink-0',
                      counts[key] === 0 || isPending
                        ? 'border-slate-700 text-slate-600 cursor-not-allowed'
                        : 'border-violet-500/50 text-violet-400 hover:bg-violet-500/10 cursor-pointer'
                    )}
                  >
                    Retry
                  </button>
                </div>
              ))}
            </div>
            <Tooltip.Arrow className="fill-slate-800" />
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
  )
}
