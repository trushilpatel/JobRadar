import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { DataTable } from '@/components/ui/data-table'
import { cn } from '@/lib/utils/utils'
import type { ResumeMatchOption } from './apply-queue-columns'

const KEYWORD_DISPLAY_LIMIT = 20

function formatKeywordList(keywords: string[]): string {
  if (keywords.length <= KEYWORD_DISPLAY_LIMIT) return keywords.join(', ')
  const shown = keywords.slice(0, KEYWORD_DISPLAY_LIMIT)
  return `${shown.join(', ')} +${keywords.length - KEYWORD_DISPLAY_LIMIT} more`
}

function MatchBar({ matched, total }: { matched: number; total: number }) {
  const pct = total > 0 ? Math.round((matched / total) * 100) : 0
  return (
    <div
      className="h-1.5 w-14 shrink-0 overflow-hidden rounded-full bg-subtle"
      role="img"
      aria-label={`${matched} of ${total} keywords matched`}
    >
      <div className="h-full bg-violet" style={{ width: `${pct}%` }} />
    </div>
  )
}

interface ResumePickerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  jobTitle: string
  jobHasKeywords: boolean
  currentResumeId: string | null
  options: ResumeMatchOption[]
  onConfirm: (resumeId: string) => void
  onClear: () => void
  isPending: boolean
}

export function ResumePickerDialog({
  open,
  onOpenChange,
  jobTitle,
  jobHasKeywords,
  currentResumeId,
  options,
  onConfirm,
  onClear,
  isPending,
}: ResumePickerDialogProps) {
  // `selectedId` starts at the committed attachment (not the best client-side
  // auto-pick) — highlighting the auto-pick by default would make a plain
  // "open then confirm" click perform a real write that wasn't previously a
  // persisted attachment (the auto-pick is display-only elsewhere in this
  // feature; see useApplyQueueResumeMatch.ts).
  const [selectedId, setSelectedId] = useState<string | null>(currentResumeId)

  const isOrphaned = currentResumeId !== null && !options.some((o) => o.id === currentResumeId)
  const canConfirm = selectedId !== null && options.some((o) => o.id === selectedId) && !isPending

  function handleConfirm() {
    if (canConfirm && selectedId) onConfirm(selectedId)
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onOpenChange(false)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="text-primary">Select resume for {jobTitle}</DialogTitle>
          <DialogDescription className="sr-only">
            Choose which resume to attach to this job, ranked by keyword match.
          </DialogDescription>
        </DialogHeader>

        {isOrphaned && (
          <p className="text-xs text-warn">
            The previously attached resume was deleted — pick a new one below.
          </p>
        )}

        {options.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted">
            No resumes uploaded yet — add one from the Resume Library.
          </p>
        ) : (
          <div
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                handleConfirm()
              }
            }}
          >
            <DataTable
              columns={[
                {
                  id: 'resume',
                  header: 'Resume',
                  cell: ({ row }) => {
                    const opt = row.original
                    const hasBreakdown = jobHasKeywords && opt.hasExtractedText && opt.total > 0
                    return (
                      <div className="flex flex-col gap-1.5 py-1">
                        <div className="flex items-center gap-3">
                          <span className="flex-1 truncate text-sm font-medium text-primary">
                            {opt.label}
                          </span>
                          {!jobHasKeywords ? (
                            <span className="text-xs text-placeholder">no keywords to match</span>
                          ) : !opt.hasExtractedText ? (
                            <span className="text-xs text-placeholder">no extracted text</span>
                          ) : (
                            <>
                              <MatchBar matched={opt.matched} total={opt.total} />
                              <span className="w-10 shrink-0 text-right font-mono text-xs text-secondary">
                                {opt.matched}/{opt.total}
                              </span>
                            </>
                          )}
                        </div>
                        {/* Missing keywords are always visible (not gated behind a
                            click) so every resume can be compared at a glance. */}
                        {hasBreakdown && opt.missingKeywords.length > 0 && (
                          <div className="text-xs">
                            <span className="text-muted">✗ missing: </span>
                            <span className="text-muted">
                              {formatKeywordList(opt.missingKeywords)}
                            </span>
                          </div>
                        )}
                      </div>
                    )
                  },
                },
              ]}
              data={options}
              getRowId={(o) => o.id}
              selectedId={selectedId ?? undefined}
              onRowClick={(o) => setSelectedId(o.id)}
              getRowClassName={(o) =>
                cn(
                  'cursor-pointer',
                  o.id === selectedId ? 'bg-[var(--accent-dim)]' : 'hover:bg-hover'
                )
              }
            />
          </div>
        )}

        <DialogFooter className="gap-2 sm:gap-2">
          {currentResumeId !== null && (
            <button
              type="button"
              disabled={isPending}
              onClick={onClear}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-muted hover:text-primary disabled:opacity-50"
            >
              Clear selection
            </button>
          )}
          <button
            type="button"
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-panel px-3 py-1.5 text-sm text-secondary hover:bg-subtle disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!canConfirm}
            onClick={handleConfirm}
            className="inline-flex items-center gap-1.5 rounded-lg bg-violet px-3 py-1.5 text-sm text-white hover:opacity-90 disabled:opacity-50"
          >
            {isPending ? 'Saving…' : 'Use this resume'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
