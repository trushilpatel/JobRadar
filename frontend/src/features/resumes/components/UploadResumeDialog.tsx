import { useState, useRef } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useResumeActions } from '../hooks/useResumeActions'
import { RESUME_MAX_FILE_BYTES, RESUME_ALLOWED_EXTENSIONS } from '../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// Builds a case-insensitive "strip any allowed extension" regex from the
// shared constant, so the accept filter and the label-fallback stripping can
// never drift apart the way two independently hardcoded lists could.
const STRIP_EXTENSION_RE = new RegExp(
  `(${RESUME_ALLOWED_EXTENSIONS.map((ext) => ext.replace('.', '\\.')).join('|')})$`,
  'i'
)

export function UploadResumeDialog({ open, onOpenChange }: Props) {
  const { upload } = useResumeActions()
  const [file, setFile] = useState<File | null>(null)
  const [label, setLabel] = useState('')
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  function handleFileChange(f: File | null) {
    setError(null)
    if (f && f.size > RESUME_MAX_FILE_BYTES) {
      setError('File exceeds the 2MB limit')
      setFile(null)
      return
    }
    setFile(f)
    if (f) setLabel((prev) => (prev ? prev : f.name.replace(STRIP_EXTENSION_RE, '')))
  }

  function reset() {
    setFile(null)
    setLabel('')
    setError(null)
    if (inputRef.current) inputRef.current.value = ''
  }

  async function handleSubmit() {
    if (!file) {
      setError('Choose a PDF or DOCX file')
      return
    }
    try {
      await upload.mutateAsync({ file, label })
      reset()
      onOpenChange(false)
    } catch {
      // upload.isError / toast already surfaces the failure — keep the dialog open so the user can retry.
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent className="bg-panel border-[var(--border)]">
        <DialogHeader>
          <DialogTitle className="text-primary">Upload resume</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div>
            <Label htmlFor="resume-file">File (PDF or DOCX, up to 2MB)</Label>
            <Input
              id="resume-file"
              ref={inputRef}
              type="file"
              accept={RESUME_ALLOWED_EXTENSIONS.join(',')}
              onChange={(e) => handleFileChange(e.target.files?.[0] ?? null)}
            />
          </div>
          <div>
            <Label htmlFor="resume-label">Label</Label>
            <Input
              id="resume-label"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g. Backend-focused v2"
            />
          </div>
          {error && <p className="text-xs text-red-400">{error}</p>}
        </div>
        <DialogFooter>
          <button
            type="button"
            disabled={upload.isPending}
            onClick={handleSubmit}
            className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet hover:opacity-90 disabled:opacity-50 text-white"
          >
            {upload.isPending ? 'Uploading…' : 'Upload'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
