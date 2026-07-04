import { useState } from 'react'
import { FileText, Download, Trash2, Pencil } from 'lucide-react'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { Input } from '@/components/ui/input'
import { useResumeActions } from '../hooks/useResumeActions'
import type { Resume } from '../types'

export function ResumeCard({ resume }: { resume: Resume }) {
  const { download, rename, remove } = useResumeActions()
  const [editing, setEditing] = useState(false)
  const [label, setLabel] = useState(resume.label)

  async function handleDownload() {
    const url = await download.mutateAsync(resume.id)
    window.location.assign(url)
  }

  async function handleRenameSubmit() {
    if (label.trim() && label !== resume.label) {
      await rename.mutateAsync({ id: resume.id, label: label.trim() })
    }
    setEditing(false)
  }

  return (
    <div className={cn('rounded-xl border border-[var(--border)] bg-panel p-4')}>
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-primary font-medium truncate">
            <FileText size={14} className="text-muted shrink-0" />
            {editing ? (
              <Input
                autoFocus
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                onBlur={handleRenameSubmit}
                onKeyDown={(e) => e.key === 'Enter' && handleRenameSubmit()}
                className="h-6 text-sm"
              />
            ) : (
              <span className="truncate">{resume.label}</span>
            )}
          </div>
          <div className="text-[11px] text-muted mt-0.5 font-mono truncate">
            {resume.original_filename}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between text-xs text-muted">
        <span>
          {(resume.size_bytes / 1024).toFixed(0)} KB · Uploaded{' '}
          {formatRelativeTime(resume.created_at)}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setEditing(true)}
            aria-label="Rename"
            className="hover:text-primary"
          >
            <Pencil size={13} />
          </button>
          <button
            type="button"
            onClick={handleDownload}
            aria-label="Download"
            className="hover:text-primary"
          >
            <Download size={13} />
          </button>
          <button
            type="button"
            onClick={() => remove.mutate(resume.id)}
            aria-label="Delete"
            className="hover:text-red-400"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>
    </div>
  )
}
