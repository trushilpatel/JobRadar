import { useQuery } from '@tanstack/react-query'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { fetchHeatmap } from '@/api/dashboard'
import type { WeeklyCount } from '@/types'

const HEAT_COLORS = ['#1c1c1e', '#085041', '#0f6e56', '#1d9e75', '#5dcaa5']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const COLS = 26

function buildWeekGrid(sparse: WeeklyCount[]): Array<{ weekStart: string; count: number }> {
  const map = new Map(sparse.map((w) => [w.week_start, w.job_count]))

  const weeks: Array<{ weekStart: string; count: number }> = []
  const now = new Date()
  const day = now.getDay()
  const monday = new Date(now)
  monday.setDate(now.getDate() - ((day + 6) % 7))
  monday.setHours(0, 0, 0, 0)

  for (let i = 51; i >= 0; i--) {
    const d = new Date(monday)
    d.setDate(monday.getDate() - i * 7)
    const iso = d.toISOString().slice(0, 10)
    weeks.push({ weekStart: iso, count: map.get(iso) ?? 0 })
  }
  return weeks
}

function colorIndex(count: number, max: number): number {
  if (count === 0) return 0
  return Math.min(4, Math.floor((count / max) * 3.5) + 1)
}

function monthLabel(weekStart: string): string {
  return MONTHS[new Date(weekStart + 'T12:00:00Z').getMonth()]
}

function formatWeekLabel(weekStart: string, count: number): string {
  const d = new Date(weekStart + 'T12:00:00Z')
  const month = MONTHS[d.getMonth()]
  const day = d.getDate()
  const year = d.getFullYear()
  return count === 0
    ? `${month} ${day}, ${year} — no new jobs`
    : `${month} ${day}, ${year} — ${count} new job${count === 1 ? '' : 's'}`
}

export function HeatmapPanel() {
  const {
    data: sparse = [],
    isLoading,
    isError,
  } = useQuery({
    queryKey: ['dashboard-heatmap'],
    queryFn: fetchHeatmap,
    refetchInterval: 300_000,
  })

  const weeks = buildWeekGrid(sparse)
  const max = Math.max(...weeks.map((w) => w.count), 1)

  return (
    <TooltipPrimitive.Provider delayDuration={120}>
      <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
        <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-3">
          new jobs detected — all watched companies
        </p>

        {isLoading ? (
          <div className="h-16 bg-white/[0.03] rounded-lg animate-pulse" />
        ) : isError ? (
          <div className="h-16 flex items-center justify-center text-xs text-slate-500">
            Failed to load heatmap data
          </div>
        ) : (
          <>
            {/* Month labels row */}
            <div
              className="grid mb-1"
              style={{ gridTemplateColumns: `repeat(${COLS}, 1fr)`, gap: '3px' }}
            >
              {Array.from({ length: COLS }, (_, col) => {
                const label = col % 4 === 0 ? monthLabel(weeks[col]?.weekStart ?? '') : ''
                return (
                  <div
                    key={col}
                    className="text-[10px] text-slate-600 text-center leading-none"
                    style={{ minWidth: 0 }}
                  >
                    {label}
                  </div>
                )
              })}
            </div>

            {/* Heatmap grid */}
            <div
              className="grid"
              style={{ gridTemplateColumns: `repeat(${COLS}, 1fr)`, gap: '3px' }}
            >
              {weeks.map((w, i) => {
                const ci = colorIndex(w.count, max)
                return (
                  <TooltipPrimitive.Root key={i}>
                    <TooltipPrimitive.Trigger asChild>
                      <div
                        className="rounded-[3px] cursor-pointer transition-opacity hover:opacity-75"
                        style={{ background: HEAT_COLORS[ci], aspectRatio: '1' }}
                      />
                    </TooltipPrimitive.Trigger>
                    <TooltipPrimitive.Portal>
                      <TooltipPrimitive.Content
                        side="top"
                        sideOffset={6}
                        className="z-50 rounded-md bg-[#1e1e23] border border-white/[0.1] px-2.5 py-1.5 text-[11px] text-slate-200 shadow-xl"
                      >
                        {formatWeekLabel(w.weekStart, w.count)}
                        <TooltipPrimitive.Arrow className="fill-[#1e1e23]" />
                      </TooltipPrimitive.Content>
                    </TooltipPrimitive.Portal>
                  </TooltipPrimitive.Root>
                )
              })}
            </div>

            {/* Legend */}
            <div className="flex items-center justify-end mt-3 gap-1.5">
              <span className="text-[11px] text-slate-600">fewer</span>
              <div className="flex gap-[3px]">
                {HEAT_COLORS.map((c, i) => (
                  <span
                    key={i}
                    className="w-[14px] h-[14px] rounded-[3px] inline-block"
                    style={{ background: c }}
                  />
                ))}
              </div>
              <span className="text-[11px] text-slate-600">more</span>
            </div>
          </>
        )}
      </div>
    </TooltipPrimitive.Provider>
  )
}
