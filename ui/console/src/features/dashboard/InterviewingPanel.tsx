import { useRef, useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { fetchInterviews, updateApplicationStage } from '@/api/dashboard'
import type { InterviewRow } from '@/types'
import { cn } from '@/lib/utils'
import { dashboardQueryKeys } from './queryKeys'

// Stage options shown in the dropdown
const STAGES: { key: string; label: string; dotColor: string; pillClass: string }[] = [
  {
    key: 'applied',
    label: 'Waiting',
    dotColor: '#888780',
    pillClass: 'bg-white/[0.06] text-slate-400',
  },
  {
    key: 'phone_screen',
    label: 'Interviewing',
    dotColor: '#7f77dd',
    pillClass: 'bg-violet-500/15 text-violet-300',
  },
  {
    key: 'technical',
    label: 'Interviewing',
    dotColor: '#7f77dd',
    pillClass: 'bg-violet-500/15 text-violet-300',
  },
  {
    key: 'final_round',
    label: 'Interviewing',
    dotColor: '#7f77dd',
    pillClass: 'bg-violet-500/15 text-violet-300',
  },
  {
    key: 'offer',
    label: 'Offer received',
    dotColor: '#1d9e75',
    pillClass: 'bg-emerald-500/15 text-emerald-400',
  },
]

function pillConfig(stage: string) {
  return (
    STAGES.find((s) => s.key === stage) ?? {
      key: stage,
      label: stage,
      dotColor: '#888780',
      pillClass: 'bg-white/[0.06] text-slate-400',
    }
  )
}

// The unique display-stage options for the dropdown
const DROPDOWN_OPTIONS = [
  { key: 'applied', label: 'Waiting', dotColor: '#888780' },
  { key: 'phone_screen', label: 'Interviewing', dotColor: '#7f77dd' },
  { key: 'offer', label: 'Offer received', dotColor: '#1d9e75' },
]

function StageDropdown({
  row,
  onStageChange,
}: {
  row: InterviewRow
  onStageChange: (id: number, stage: string) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const cfg = pillConfig(row.stage)

  useEffect(() => {
    if (!open) return
    function onClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClickOutside)
    return () => document.removeEventListener('mousedown', onClickOutside)
  }, [open])

  return (
    <div ref={ref} className="relative shrink-0">
      <button
        onClick={(e) => {
          e.stopPropagation()
          setOpen((v) => !v)
        }}
        className={cn(
          'inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-medium border-0 cursor-pointer hover:opacity-85 transition-opacity',
          cfg.pillClass
        )}
      >
        {cfg.label}
        <ChevronDown size={11} className="opacity-60" />
      </button>

      {open && (
        <div className="absolute right-0 top-[26px] z-20 bg-[#141418] border border-white/[0.12] rounded-lg shadow-xl min-w-[160px] overflow-hidden">
          {DROPDOWN_OPTIONS.map((opt) => (
            <button
              key={opt.key}
              onClick={() => {
                onStageChange(row.application_id, opt.key)
                setOpen(false)
              }}
              className={cn(
                'w-full flex items-center gap-2 px-3 py-2 text-[11px] text-left transition-colors',
                row.stage === opt.key
                  ? 'bg-white/[0.06] text-white'
                  : 'text-slate-300 hover:bg-white/[0.05]'
              )}
            >
              <span
                className="w-[6px] h-[6px] rounded-full shrink-0"
                style={{ background: opt.dotColor }}
              />
              {opt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export function InterviewingPanel() {
  const queryClient = useQueryClient()

  const { data: rows, isLoading } = useQuery({
    queryKey: dashboardQueryKeys.interviews(),
    queryFn: fetchInterviews,
    refetchInterval: 30_000,
  })

  const mutation = useMutation({
    mutationFn: ({ id, stage }: { id: number; stage: string }) => updateApplicationStage(id, stage),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.interviews() })
    },
  })

  return (
    <div className="bg-white/[0.04] border border-white/[0.07] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-slate-500 mb-4">
        interviewing
      </p>

      {isLoading || !rows ? (
        <div className="space-y-3 animate-pulse">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="h-10 bg-white/[0.04] rounded-lg" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <p className="text-sm text-slate-600 py-2">No active applications</p>
      ) : (
        <div>
          {rows.map((row, i) => (
            <div
              key={row.application_id}
              className={cn(
                'flex items-center gap-3 py-2.5',
                i < rows.length - 1 && 'border-b border-white/[0.05]'
              )}
            >
              <div className="flex-1 min-w-0">
                <p className="text-[13px] font-medium text-white truncate">{row.company_name}</p>
                <p className="text-[11px] text-slate-500 truncate mt-0.5">{row.job_title}</p>
              </div>
              <StageDropdown
                row={row}
                onStageChange={(id, stage) => mutation.mutate({ id, stage })}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
