import { useState } from 'react'
import { toast } from 'sonner'
import { Save, Lock } from 'lucide-react'
import { cn } from '@/lib/utils/utils'

interface Props {
  content: string
  readOnly?: boolean
  className?: string
  // When set, "Save" shows a dirty indicator and persists into local state
  // only. The mockup doesn't write to a backend.
  onSave?: (next: string) => void
}

export function ResumeContentPane({ content, readOnly, className, onSave }: Props) {
  const [value, setValue] = useState(content)
  const [prevContent, setPrevContent] = useState(content)
  const [dirty, setDirty] = useState(false)

  // React 19 idiom for "reset local state when a prop changes": adjust state
  // during render. React discards the in-progress render and starts over.
  if (content !== prevContent) {
    setPrevContent(content)
    setValue(content)
    setDirty(false)
  }

  const save = () => {
    onSave?.(value)
    setDirty(false)
    toast.success('Saved as new version (mockup)')
  }

  return (
    <div
      className={cn(
        'rounded-xl border border-[var(--border)] bg-panel flex flex-col min-h-0',
        className
      )}
    >
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-[var(--border)]">
        <div className="flex items-center gap-2 text-xs text-muted">
          {readOnly ? <Lock size={12} /> : <Save size={12} />}
          <span>{readOnly ? 'Read-only (submitted version)' : 'Markdown / plain text'}</span>
          {dirty && !readOnly && (
            <span className="text-amber-300 text-[10px]">• unsaved changes</span>
          )}
        </div>
        {!readOnly && (
          <button
            onClick={save}
            disabled={!dirty}
            className={cn(
              'text-xs px-2.5 py-1 rounded transition-colors',
              dirty
                ? 'bg-violet-600 hover:bg-violet-500 text-white'
                : 'bg-subtle text-muted cursor-not-allowed'
            )}
          >
            Save version
          </button>
        )}
      </div>
      <textarea
        value={value}
        readOnly={readOnly}
        onChange={(e) => {
          setValue(e.target.value)
          setDirty(true)
        }}
        spellCheck={false}
        className={cn(
          'flex-1 w-full p-4 text-sm leading-relaxed text-primary font-mono bg-transparent',
          'resize-none outline-none focus:bg-subtle transition-colors',
          'min-h-[420px]'
        )}
      />
    </div>
  )
}
