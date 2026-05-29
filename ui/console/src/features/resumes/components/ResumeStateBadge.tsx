import { cn } from '@/lib/utils/utils'
import type { ResumeAttachmentState } from '../types'

const STATE_STYLE: Record<ResumeAttachmentState, string> = {
  draft: 'bg-amber-500/10 text-amber-300 ring-1 ring-amber-500/20',
  final: 'bg-emerald-500/10 text-emerald-300 ring-1 ring-emerald-500/20',
  submitted: 'bg-violet-500/10 text-violet-300 ring-1 ring-violet-500/20',
}

const STATE_LABEL: Record<ResumeAttachmentState, string> = {
  draft: 'Draft',
  final: 'Final',
  submitted: 'Submitted',
}

export function ResumeStateBadge({ state }: { state: ResumeAttachmentState }) {
  return (
    <span
      className={cn(
        'inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium',
        STATE_STYLE[state]
      )}
    >
      {STATE_LABEL[state]}
    </span>
  )
}
