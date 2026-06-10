import * as Select from '@radix-ui/react-select'
import { ChevronDown, FileText, Check } from 'lucide-react'
import { toast } from 'sonner'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils/utils'
import { routes } from '@/config/routes'
import { useResumes } from '../hooks/useResumes'
import { useResume } from '../hooks/useResume'

interface Props {
  // The job we're attaching to. Reserved for future hooks (e.g. fetching
  // attachment state); currently unused inside the picker — caller passes it
  // so the API is symmetric with the rest of the resume attach surface.
  jobId: string
  // The currently selected resume id, or null if no attachment yet.
  selectedResumeId: string | null
  // Called when the user picks a resume from the dropdown. The caller decides
  // what to do with the selection (attach as draft via TanStack mutation, etc.)
  onSelect: (resumeId: string) => void
  // Visual variant.
  size?: 'sm' | 'md'
  className?: string
}

// Inline resume picker used in:
//   - Apply Queue table rows (the Resume column)
//   - JobDetailPanel "Resume" section
//
// Renders a Radix Select trigger styled to fit a table cell. When no resumes
// exist anywhere, the trigger is disabled and the caller is responsible for
// rendering the "No resume found / Insert resume for this" CTA next to it
// (so the trigger stays a single concern).
export function ResumePickerInline({
  jobId,
  selectedResumeId,
  onSelect,
  size = 'sm',
  className,
}: Props) {
  // Hold onto jobId via a void-noop so eslint doesn't flag the prop and so
  // callers don't need to change when we wire jobId into real mutations.
  void jobId
  const { data: resumes } = useResumes()
  const list = resumes ?? []
  const empty = list.length === 0

  const selected = useResume(selectedResumeId ?? undefined)
  const selectedName = selected.data?.resume?.name

  return (
    <Select.Root
      value={selectedResumeId ?? undefined}
      onValueChange={(v) => {
        onSelect(v)
        const picked = list.find((r) => r.id === v)
        toast.success(`Attached: ${picked?.name ?? 'resume'} (draft)`)
      }}
      disabled={empty}
    >
      <Select.Trigger
        className={cn(
          'flex items-center gap-1.5 rounded-lg border outline-none cursor-pointer transition-colors',
          size === 'sm' ? 'text-xs px-2 py-1' : 'text-sm px-2.5 py-1.5',
          empty
            ? 'border-[var(--border)] bg-subtle text-muted cursor-not-allowed'
            : selectedResumeId
              ? 'border-violet-500/30 bg-violet-500/[0.06] text-violet-200 hover:bg-violet-500/[0.1]'
              : 'border-[var(--border-strong)] bg-subtle text-secondary hover:bg-hover',
          className
        )}
      >
        <FileText size={11} className="shrink-0 opacity-70" />
        <Select.Value
          placeholder={empty ? 'No resume found' : 'Select resume'}
          className="truncate"
        >
          {selectedName && <span className="truncate">{selectedName}</span>}
        </Select.Value>
        <Select.Icon className="ml-auto">
          <ChevronDown size={11} />
        </Select.Icon>
      </Select.Trigger>

      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-50 min-w-[220px] max-h-[340px] rounded-lg border border-[var(--border-strong)] bg-panel shadow-xl overflow-hidden"
        >
          <Select.Viewport className="p-1">
            {list.map((r) => (
              <Select.Item
                key={r.id}
                value={r.id}
                className={cn(
                  'flex items-center justify-between gap-3 text-xs px-2.5 py-2 rounded outline-none cursor-pointer',
                  'text-primary data-[highlighted]:bg-violet-500/[0.12] data-[highlighted]:text-primary'
                )}
              >
                <Select.ItemText>
                  <div className="min-w-0">
                    <div className="truncate">{r.name}</div>
                    <div className="text-[10px] text-muted mt-0.5">
                      v{r.version_count} · {r.attached_job_count} attached
                    </div>
                  </div>
                </Select.ItemText>
                <Select.ItemIndicator className="shrink-0 text-violet-300">
                  <Check size={12} />
                </Select.ItemIndicator>
              </Select.Item>
            ))}
            <div className="border-t border-[var(--border)] mt-1 pt-1 px-1">
              <Link
                to={routes.resumeLibrary()}
                className="block text-[11px] text-muted hover:text-primary px-2 py-1.5 rounded hover:bg-hover"
              >
                Manage resumes →
              </Link>
            </div>
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}
