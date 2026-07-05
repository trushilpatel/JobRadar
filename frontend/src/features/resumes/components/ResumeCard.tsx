import { useState } from 'react'
import { FileText, Download, Trash2, Pencil, Sparkles } from 'lucide-react'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { useResumeActions } from '../hooks/useResumeActions'
import { useExtractResumeKeywords } from '../hooks/useExtractResumeKeywords'
import { RESUME_KEYWORDS_VISIBLE_LIMIT } from '../types'
import type { Resume } from '../types'

// Triggers a browser "save as" for an in-memory Blob via a throwaway <a
// download> element — the standard DOM pattern for saving fetched bytes
// without a server redirect. The object URL is revoked on the next tick
// (not synchronously) since some browsers cancel the save if revoked before
// they've had a chance to start reading it.
function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function ResumeCard({ resume }: { resume: Resume }) {
  const { download, rename, remove } = useResumeActions()
  const extractKeywords = useExtractResumeKeywords()
  const [editing, setEditing] = useState(false)
  const [label, setLabel] = useState(resume.label)

  const visibleKeywords = resume.keywords.slice(0, RESUME_KEYWORDS_VISIBLE_LIMIT)
  const hiddenKeywordCount = resume.keywords.length - visibleKeywords.length

  async function handleDownload() {
    const blob = await download.mutateAsync(resume.id)
    saveBlob(blob, resume.original_filename)
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

      {visibleKeywords.length > 0 && (
        <div className="flex flex-wrap gap-1 mb-3">
          {visibleKeywords.map((keyword) => (
            <Badge key={keyword} variant="mono">
              {keyword}
            </Badge>
          ))}
          {hiddenKeywordCount > 0 && <Badge variant="ghost">+{hiddenKeywordCount} more</Badge>}
        </div>
      )}

      <div className="flex items-center justify-between text-xs text-muted">
        <span>
          {(resume.size_bytes / 1024).toFixed(0)} KB · Uploaded{' '}
          {formatRelativeTime(resume.created_at)}
        </span>
        <div className="flex items-center gap-2">
          {resume.keywords.length === 0 && (
            <button
              type="button"
              onClick={() => extractKeywords.mutate(resume.id)}
              disabled={extractKeywords.isPending}
              aria-label="Extract keywords"
              className="hover:text-primary disabled:opacity-50"
            >
              <Sparkles size={13} />
            </button>
          )}
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
