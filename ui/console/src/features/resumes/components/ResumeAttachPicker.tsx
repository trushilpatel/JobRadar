import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import * as Dialog from '@radix-ui/react-dialog'
import { Paperclip, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { routes } from '@/lib/routes'
import { useResumes } from '@/features/resumes/hooks/useResumes'

interface Props {
  // The job to attach a resume to.
  jobId: string
  jobTitle: string
  // Renders the trigger button — caller controls icon/label/style.
  triggerLabel?: string
}

// The resume picker. In the real app this also gets embedded into
// JobDetailPanel ("attach resume") and into ApplyQueuePage's
// "+ attach resume" buttons on unattached saved jobs.
export function ResumeAttachPicker({ jobId, jobTitle, triggerLabel = 'Attach resume' }: Props) {
  const [open, setOpen] = useState(false)
  const { data } = useResumes()
  const resumes = data ?? []
  const navigate = useNavigate()

  const pick = (resumeId: string) => {
    toast.success('Resume attached as draft')
    setOpen(false)
    // Side-by-side editor with the job context loaded on the right pane.
    navigate(routes.resumeEdit(resumeId, { jobId }))
  }

  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Trigger asChild>
        <button
          className={cn(
            'inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded',
            'bg-violet-600/20 hover:bg-violet-600/30 text-violet-300 ring-1 ring-violet-500/30'
          )}
        >
          <Paperclip size={11} />
          {triggerLabel}
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-[#141420] border border-white/[0.08] rounded-xl shadow-xl">
          <div className="flex items-start justify-between gap-4 p-5 border-b border-white/[0.05]">
            <div className="min-w-0">
              <Dialog.Title className="text-base font-semibold text-white">
                Attach resume
              </Dialog.Title>
              <Dialog.Description className="text-xs text-slate-400 mt-1 truncate">
                For: {jobTitle}
              </Dialog.Description>
            </div>
            <Dialog.Close className="text-slate-400 hover:text-white">
              <X size={16} />
            </Dialog.Close>
          </div>
          <div className="max-h-[380px] overflow-y-auto p-2">
            {resumes.length === 0 && (
              <div className="text-sm text-slate-400 px-3 py-4 text-center">
                No resumes yet — upload one first.
              </div>
            )}
            {resumes.map((r) => (
              <button
                key={r.id}
                onClick={() => pick(r.id)}
                className="w-full text-left rounded-lg px-3 py-2.5 hover:bg-white/[0.04] transition-colors"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-sm text-white truncate">{r.name}</div>
                    <div className="text-[11px] text-slate-500 font-mono mt-0.5">{r.label}</div>
                  </div>
                  <div className="text-[10px] text-slate-500 shrink-0 whitespace-nowrap">
                    v{r.version_count} · {r.attached_job_count} attached
                  </div>
                </div>
              </button>
            ))}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
