import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchHeatmap } from '@/api/dashboard'
import type { WeeklyCount } from '@/types'

// Dark-theme heat colours: empty → low → max
const HEAT_COLORS = ['#1c1c1e', '#085041', '#0f6e56', '#1d9e75', '#5dcaa5']

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const COLS = 26 // 26 columns × 2 rows = 52 weeks

function buildWeekGrid(sparse: WeeklyCount[]): Array<{ weekStart: string; count: number }> {
  // Build a map from week_start ISO date → count
  const map = new Map(sparse.map((w) => [w.week_start, w.job_count]))

  // Generate all 52 week-start dates going back from current week
  const weeks: Array<{ weekStart: string; count: number }> = []
  const now = new Date()
  // Find Monday of current week
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

export function HeatmapPanel() {
  const [tooltip, setTooltip] = useState<string | null>(null)

  const { data: sparse = [], isLoading } = useQuery({
    queryKey: ['dashboard-heatmap'],
    queryFn: fetchHeatmap,
    refetchInterval: 300_000,
  })

  const weeks = buildWeekGrid(sparse)
  const max = Math.max(...weeks.map((w) => w.count), 1)

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-3">
        hiring activity — all watched companies
      </p>

      {isLoading ? (
        <div className="h-16 bg-white/[0.03] rounded-lg animate-pulse" />
      ) : (
        <>
          {/* Month labels row */}
          <div
            className="grid mb-1"
            style={{ gridTemplateColumns: `repeat(${COLS}, 1fr)`, gap: '3px' }}
          >
            {Array.from({ length: COLS }, (_, col) => {
              // Show month label at col 0, 4, 8, 12, 16, 20, 24 (every 4 cols)
              const weekIndex = col // first row only for labels
              const label = col % 4 === 0 ? monthLabel(weeks[weekIndex]?.weekStart ?? '') : ''
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

          {/* Heatmap grid: 26 cols × 2 rows = 52 weeks */}
          <div className="grid" style={{ gridTemplateColumns: `repeat(${COLS}, 1fr)`, gap: '3px' }}>
            {weeks.map((w, i) => {
              const ci = colorIndex(w.count, max)
              const label = `${w.weekStart} — ${w.count} new jobs`
              return (
                <div
                  key={i}
                  className="rounded-[3px] cursor-pointer transition-opacity hover:opacity-75"
                  style={{
                    background: HEAT_COLORS[ci],
                    aspectRatio: '1',
                  }}
                  onMouseEnter={() => setTooltip(label)}
                  onMouseLeave={() => setTooltip(null)}
                />
              )
            })}
          </div>

          {/* Footer: tooltip + legend */}
          <div className="flex items-center justify-between mt-3">
            <span className="text-[12px] text-slate-500 min-h-[18px]">
              {tooltip ?? 'hover any week to see details'}
            </span>
            <div className="flex items-center gap-1.5">
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
          </div>
        </>
      )}
    </div>
  )
}
