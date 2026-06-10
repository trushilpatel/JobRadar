import type { CSSProperties } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { fetchHeatmap } from '../api/dashboard.api'
import type { WeeklyCount } from '@/types'
import { dashboardQueryKeys } from '../queryKeys'

const HEAT_COLORS = ['#1c1c1e', '#085041', '#0f6e56', '#1d9e75', '#5dcaa5']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const HALF = 26

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
  return MONTHS[new Date(weekStart + 'T12:00:00Z').getMonth()] ?? ''
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

function HeatRow({
  slice,
  max,
  offset,
  prevLastLabelMonth = '',
}: {
  slice: Array<{ weekStart: string; count: number }>
  max: number
  offset: number
  // When provided, suppress the col-0 label if it duplicates the last
  // labeled month from the previous row (prevents "Sep … Sep" split).
  prevLastLabelMonth?: string
}) {
  return (
    <div className="space-y-1">
      {/* Month labels */}
      <div className="grid grid-cols-[repeat(26,1fr)] gap-[3px]">
        {slice.map((w, col) => {
          const label =
            col % 4 === 0 && !(col === 0 && monthLabel(w.weekStart) === prevLastLabelMonth)
              ? monthLabel(w.weekStart)
              : ''
          return (
            <div
              key={col}
              className="text-[10px] text-placeholder text-center leading-none min-w-0"
            >
              {label}
            </div>
          )
        })}
      </div>

      {/* Cells */}
      <div className="grid grid-cols-[repeat(26,1fr)] gap-[3px]">
        {slice.map((w, col) => {
          const ci = colorIndex(w.count, max)
          return (
            <TooltipPrimitive.Root key={offset + col}>
              <TooltipPrimitive.Trigger asChild>
                <div
                  style={{ '--cell-bg': HEAT_COLORS[ci] } as CSSProperties}
                  className="rounded-[3px] cursor-pointer transition-opacity hover:opacity-75 aspect-square [background:var(--cell-bg)]"
                />
              </TooltipPrimitive.Trigger>
              <TooltipPrimitive.Portal>
                <TooltipPrimitive.Content
                  side="top"
                  sideOffset={6}
                  className="z-50 rounded-md bg-subtle border border-[var(--border-strong)] px-2.5 py-1.5 text-[11px] text-primary shadow-xl"
                >
                  {formatWeekLabel(w.weekStart, w.count)}
                  <TooltipPrimitive.Arrow className="fill-subtle" />
                </TooltipPrimitive.Content>
              </TooltipPrimitive.Portal>
            </TooltipPrimitive.Root>
          )
        })}
      </div>
    </div>
  )
}

export function HeatmapPanel() {
  const {
    data: sparse,
    isLoading,
    isError,
  } = useQuery({
    queryKey: dashboardQueryKeys.heatmap(),
    queryFn: fetchHeatmap,
    refetchInterval: 300_000,
  })

  const weeks = buildWeekGrid(sparse ?? [])
  const max = Math.max(...weeks.map((w) => w.count), 1)
  const firstHalf = weeks.slice(0, HALF)
  const secondHalf = weeks.slice(HALF)

  // The last labeled column in firstHalf is at col 24 (largest multiple of 4
  // within [0, HALF-1]=25). Compare it with secondHalf[0] to suppress the
  // duplicate month label that appears when both rows start in the same month.
  const lastLabelMonthInFirst = firstHalf[24] ? monthLabel(firstHalf[24].weekStart) : ''

  return (
    <TooltipPrimitive.Provider delayDuration={120}>
      <div className="bg-subtle border border-[var(--border)] rounded-xl p-5">
        <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-muted mb-3">
          new jobs detected — all watched companies
        </p>

        {isLoading ? (
          <div className="h-24 bg-subtle rounded-lg animate-pulse" />
        ) : isError ? (
          <div className="h-24 flex items-center justify-center text-xs text-muted">
            Failed to load heatmap data
          </div>
        ) : (
          <div className="space-y-3">
            <HeatRow slice={firstHalf} max={max} offset={0} />
            <HeatRow
              slice={secondHalf}
              max={max}
              offset={HALF}
              prevLastLabelMonth={lastLabelMonthInFirst}
            />

            {/* Legend */}
            <div className="flex items-center justify-end gap-1.5">
              <span className="text-[11px] text-placeholder">fewer</span>
              <div className="flex gap-[3px]">
                {HEAT_COLORS.map((c, i) => (
                  <span
                    key={i}
                    style={{ '--cell-bg': c } as CSSProperties}
                    className="w-[14px] h-[14px] rounded-[3px] inline-block [background:var(--cell-bg)]"
                  />
                ))}
              </div>
              <span className="text-[11px] text-placeholder">more</span>
            </div>
          </div>
        )}
      </div>
    </TooltipPrimitive.Provider>
  )
}
