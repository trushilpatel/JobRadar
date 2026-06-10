import type { CSSProperties } from 'react'
import * as Select from '@radix-ui/react-select'
import { ChevronDown, Check } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { fetchInterviews } from '../api/dashboard.api'
import type { InterviewRow } from '@/types'
import { cn } from '@/lib/utils/utils'
import { dashboardQueryKeys } from '../queryKeys'
import { useInterviewStage } from '../hooks/useInterviewStage'

const DOT_COLOR_WAITING = '#888780'
const DOT_COLOR_INTERVIEWING = '#7f77dd'
const DOT_COLOR_OFFER = '#1d9e75'

const STAGES = [
  {
    key: 'applied',
    label: 'Waiting',
    dotColor: DOT_COLOR_WAITING,
    pillClass: 'bg-subtle text-muted',
  },
  {
    key: 'phone_screen',
    label: 'Interviewing',
    dotColor: DOT_COLOR_INTERVIEWING,
    pillClass: 'bg-violet-500/15 text-violet-300',
  },
  {
    key: 'offer',
    label: 'Offer received',
    dotColor: DOT_COLOR_OFFER,
    pillClass: 'bg-emerald-500/15 text-emerald-400',
  },
]

function pillConfig(stage: string) {
  return (
    STAGES.find((s) => s.key === stage) ?? {
      key: stage,
      label: stage,
      dotColor: DOT_COLOR_WAITING,
      pillClass: 'bg-subtle text-muted',
    }
  )
}

function StageSelect({
  row,
  onStageChange,
}: {
  row: InterviewRow
  onStageChange: (id: number, stage: string) => void
}) {
  const cfg = pillConfig(row.stage)
  return (
    <Select.Root value={row.stage} onValueChange={(val) => onStageChange(row.application_id, val)}>
      <Select.Trigger
        className={cn(
          'inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-medium cursor-pointer hover:opacity-85 transition-opacity focus:outline-none focus-visible:ring-1 focus-visible:ring-white/30',
          cfg.pillClass
        )}
        aria-label={`Stage: ${cfg.label}`}
      >
        <Select.Value>{cfg.label}</Select.Value>
        <Select.Icon>
          <ChevronDown size={11} className="opacity-60" />
        </Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <Select.Content
          className="z-50 min-w-[160px] overflow-hidden bg-panel border border-[var(--border-strong)] rounded-lg shadow-xl"
          position="popper"
          sideOffset={4}
        >
          <Select.Viewport className="p-1">
            {STAGES.map((opt) => (
              <Select.Item
                key={opt.key}
                value={opt.key}
                className="flex items-center gap-2 px-3 py-2 text-[11px] text-secondary rounded cursor-pointer hover:bg-hover focus:bg-hover focus:outline-none data-[state=checked]:text-primary data-[state=checked]:bg-hover"
              >
                <span
                  style={{ '--dot-bg': opt.dotColor } as CSSProperties}
                  className="w-[6px] h-[6px] rounded-full shrink-0 [background:var(--dot-bg)]"
                />
                <Select.ItemText>{opt.label}</Select.ItemText>
                <Select.ItemIndicator className="ml-auto">
                  <Check size={11} />
                </Select.ItemIndicator>
              </Select.Item>
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}

export function InterviewingPanel() {
  const {
    data: rows,
    isLoading,
    isError,
  } = useQuery({
    queryKey: dashboardQueryKeys.interviews(),
    queryFn: fetchInterviews,
    refetchInterval: 30_000,
  })
  const mutation = useInterviewStage()

  return (
    <div className="bg-subtle border border-[var(--border)] rounded-xl p-5">
      <p className="text-[10px] font-medium uppercase tracking-[0.07em] text-muted mb-4">
        interviewing
      </p>

      {isLoading ? (
        <div className="space-y-3 animate-pulse">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-10 bg-subtle rounded-lg" />
          ))}
        </div>
      ) : isError ? (
        <p className="text-sm text-muted py-2">Failed to load interviews</p>
      ) : !rows || rows.length === 0 ? (
        <p className="text-sm text-placeholder py-2">No active applications</p>
      ) : (
        <div>
          {rows.map((row, i) => (
            <div
              key={row.application_id}
              className={cn(
                'flex items-center gap-3 py-2.5',
                i < rows.length - 1 && 'border-b border-[var(--border)]'
              )}
            >
              <div className="flex-1 min-w-0">
                <p className="text-[13px] font-medium text-primary truncate">{row.company_name}</p>
                <p className="text-[11px] text-muted truncate mt-0.5">{row.job_title}</p>
              </div>
              <StageSelect
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
