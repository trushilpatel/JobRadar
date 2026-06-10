import { cn, formatRelativeTime } from '@/lib/utils/utils'
import type { ResumeVersion } from '../types'

interface Props {
  versions: ResumeVersion[]
  activeId: string
  onSelect?: (id: string) => void
  selectedId?: string
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}

export function ResumeVersionList({ versions, activeId, onSelect, selectedId }: Props) {
  const ordered = [...versions].sort((a, b) => b.version_number - a.version_number)
  return (
    <div className="space-y-1">
      {ordered.map((v) => {
        const isActive = v.id === activeId
        const isSelected = selectedId ? v.id === selectedId : isActive
        return (
          <button
            key={v.id}
            onClick={() => onSelect?.(v.id)}
            className={cn(
              'w-full text-left rounded-lg px-3 py-2.5 transition-colors border',
              isSelected
                ? 'border-violet-500/30 bg-violet-500/5'
                : 'border-[var(--border)] bg-panel hover:bg-subtle'
            )}
          >
            <div className="flex items-center justify-between mb-0.5">
              <div className="flex items-center gap-2">
                <span className="text-primary text-sm font-medium tabular-nums">
                  v{v.version_number}
                </span>
                {isActive && (
                  <span className="text-[10px] text-violet-300 bg-violet-500/10 px-1.5 py-0.5 rounded">
                    active
                  </span>
                )}
              </div>
              <span className="text-[10px] text-muted uppercase tracking-wide">
                {v.file_format}
              </span>
            </div>
            {v.change_summary && (
              <div className="text-xs text-muted mt-1 line-clamp-2">{v.change_summary}</div>
            )}
            <div className="flex items-center justify-between mt-2 text-[10px] text-muted">
              <span>{formatRelativeTime(v.created_at)}</span>
              <span>{formatBytes(v.size_bytes)}</span>
            </div>
          </button>
        )
      })}
    </div>
  )
}
