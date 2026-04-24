import { useState, useRef, useEffect } from 'react'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'

interface AdapterConfigCellProps {
  value: number
  min: number
  max: number
  unit?: string
  onSave: (value: number) => Promise<void>
}

type CellState = 'display' | 'editing' | 'saving' | 'success' | 'error'

export function AdapterConfigCell({ value, min, max, unit, onSave }: AdapterConfigCellProps) {
  const [state, setState] = useState<CellState>('display')
  const [draft, setDraft] = useState(String(value))
  const [errorMsg, setErrorMsg] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  // Sync display value when prop changes (setState-in-effect is intentional here — syncing external prop)
  useEffect(() => {
    if (state === 'display') {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDraft(String(value))
    }
  }, [value, state])

  useEffect(() => {
    if (state === 'editing') {
      inputRef.current?.select()
    }
  }, [state])

  // Brief green flash then return to display
  useEffect(() => {
    if (state === 'success') {
      const timer = setTimeout(() => setState('display'), 800)
      return () => clearTimeout(timer)
    }
  }, [state])

  function beginEdit() {
    setDraft(String(value))
    setErrorMsg('')
    setState('editing')
  }

  function validate(raw: string): number | null {
    const n = Number(raw)
    if (!Number.isInteger(n) || Number.isNaN(n)) return null
    if (n < min || n > max) return null
    return n
  }

  async function commit() {
    const parsed = validate(draft)
    if (parsed === null) {
      setErrorMsg(`Must be ${min}–${max}`)
      setState('error')
      return
    }
    if (parsed === value) {
      setState('display')
      return
    }
    setState('saving')
    try {
      await onSave(parsed)
      setState('success')
    } catch {
      setDraft(String(value))
      setErrorMsg('Save failed')
      setState('error')
    }
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      void commit()
    }
    if (e.key === 'Escape') {
      setDraft(String(value))
      setState('display')
    }
  }

  if (state === 'saving') {
    return (
      <div className="flex items-center gap-1.5 text-sm text-white/70">
        <Loader2 size={13} className="animate-spin text-violet-400" />
        <span>{draft}{unit ? ` ${unit}` : ''}</span>
      </div>
    )
  }

  if (state === 'editing' || state === 'error') {
    return (
      <div>
        <input
          ref={inputRef}
          type="number"
          value={draft}
          min={min}
          max={max}
          onChange={(e) => {
            setDraft(e.target.value)
            setErrorMsg('')
            if (state === 'error') setState('editing')
          }}
          onBlur={() => void commit()}
          onKeyDown={handleKeyDown}
          className={cn(
            'w-20 bg-white/10 border rounded px-2 py-0.5 text-sm text-white focus:outline-none transition-colors',
            state === 'error' ? 'border-red-500' : 'border-violet-500'
          )}
        />
        {unit && <span className="ml-1 text-xs text-white/50">{unit}</span>}
        {state === 'error' && errorMsg && (
          <p className="text-[11px] text-red-400 mt-0.5">{errorMsg}</p>
        )}
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={beginEdit}
      title="Click to edit"
      className={cn(
        'cursor-text text-sm transition-colors rounded px-1.5 py-0.5 -mx-1.5',
        state === 'success'
          ? 'text-emerald-400 bg-emerald-500/10'
          : 'text-white/70 hover:text-white hover:bg-white/[0.08]'
      )}
    >
      {value}
      {unit && <span className="ml-1 text-white/40 text-xs">{unit}</span>}
    </button>
  )
}
