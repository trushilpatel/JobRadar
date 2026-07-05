import { Star } from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils/utils'

// Cross-feature note: this cell only ever receives plain { id, label } pairs,
// never the resumes feature's `Resume` domain type — importing across
// features is forbidden (see features/CLAUDE.md). The pages layer (which may
// import both features) is responsible for mapping `Resume[]` down to this
// shape, the same pattern JobsTable's `getAttachmentState` callback uses to
// avoid a features/jobs -> features/resumes import.
export interface ApplyQueueResumeOption {
  id: string
  label: string
  // True when this resume is the best keyword-match for the row's job
  // (issue #264) — a hint surfaced as a "Recommended" badge, never a hard
  // gate; the caller (apply-queue-columns.tsx) computes this per row from
  // the item's recommended_resume_id.
  recommended?: boolean
}

interface ResumePickerCellProps {
  jobId: string
  resumeId: string | null
  resumeLabel: string | null
  options: ApplyQueueResumeOption[]
  onChange: (jobId: string, resumeId: string | null) => void
  disabled?: boolean
}

// Radix Select's Item value cannot be an empty string, so "no resume" is
// represented by this sentinel and translated back to `null` at the callback
// boundary — callers never see the sentinel.
const NONE_VALUE = '__none__'

export function ResumePickerCell({
  jobId,
  resumeId,
  resumeLabel,
  options,
  onChange,
  disabled,
}: ResumePickerCellProps) {
  // Defensive: the attached resume may have been deleted (or otherwise not be
  // in the caller's current options list) after being selected here. Without
  // this, Radix Select would render blank instead of the orphaned label,
  // silently hiding which resume was actually attached to the job.
  const attachedNotInOptions = resumeId !== null && !options.some((o) => o.id === resumeId)

  return (
    <Select
      value={resumeId ?? NONE_VALUE}
      onValueChange={(value) => onChange(jobId, value === NONE_VALUE ? null : value)}
      disabled={disabled}
    >
      <SelectTrigger
        aria-label={`Select resume for ${jobId}`}
        onClick={(e) => e.stopPropagation()}
        className={cn('h-7 w-full max-w-[220px] text-xs', disabled && 'opacity-60')}
      >
        <SelectValue placeholder="No resume selected" />
      </SelectTrigger>
      <SelectContent onClick={(e) => e.stopPropagation()}>
        <SelectItem value={NONE_VALUE}>
          <span className="text-placeholder">No resume selected</span>
        </SelectItem>
        {attachedNotInOptions && resumeId && (
          <SelectItem value={resumeId}>{resumeLabel ?? 'Unknown resume'}</SelectItem>
        )}
        {options.map((opt) => (
          <SelectItem key={opt.id} value={opt.id}>
            <span className="flex items-center gap-1.5">
              <span className="truncate">{opt.label}</span>
              {opt.recommended && (
                <span
                  className={cn(
                    'inline-flex items-center gap-0.5 shrink-0 rounded px-1 py-px text-[10px] font-medium leading-none text-violet',
                    'bg-[color-mix(in_srgb,var(--violet)_14%,transparent)]'
                  )}
                >
                  <Star size={9} className="fill-current" />
                  Recommended
                </span>
              )}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
