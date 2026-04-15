import { useState, useEffect, useRef } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'

interface MultiSelectDropdownProps {
  options: string[]
  selected: string[]
  onChange: (v: string[]) => void
  placeholder?: string
  singularLabel?: (n: number) => string
  searchPlaceholder?: string
  addable?: boolean
  onAdd?: (value: string) => void
  addPlaceholder?: string
  leadingIcon?: React.ReactNode
  'aria-label'?: string
}

export function MultiSelectDropdown({
  options, selected, onChange,
  placeholder = 'All',
  singularLabel = n => `${n} selected`,
  searchPlaceholder = 'Search…',
  addable, onAdd, addPlaceholder = 'Add new…',
  leadingIcon,
  ...rest
}: MultiSelectDropdownProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [newVal, setNewVal] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function onDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  useEffect(() => {
    if (!open) { setSearch(''); setNewVal('') }
  }, [open])

  const visible = options.filter(o => o.toLowerCase().includes(search.toLowerCase()))
  const label = selected.length === 0
    ? placeholder
    : selected.length === 1 ? selected[0] : singularLabel(selected.length)

  function toggle(v: string) {
    onChange(selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v])
  }

  function handleAdd() {
    const v = newVal.trim().toLowerCase()
    if (!v) return
    onAdd?.(v)
    if (!selected.includes(v)) onChange([...selected, v])
    setNewVal('')
  }

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-label={rest['aria-label']}
        aria-expanded={open}
        onClick={() => setOpen(o => !o)}
        className={cn(
          'inline-flex items-center gap-1.5 text-[11px] px-2.5 py-1.5 rounded-lg border transition-colors',
          selected.length > 0
            ? 'bg-violet-500/15 border-violet-500/35 text-violet-300'
            : 'bg-white/[0.03] border-white/[0.08] text-slate-400 hover:bg-white/[0.06]',
        )}
      >
        {leadingIcon}
        <span>{label}</span>
        <ChevronDown size={10} />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 z-30 min-w-[240px] bg-[#13131e] border border-white/[0.1] rounded-[10px] shadow-[0_8px_24px_rgba(0,0,0,0.4)] overflow-hidden">
          <input
            autoFocus
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder={searchPlaceholder}
            className="w-full bg-white/[0.03] border-b border-white/[0.08] px-2.5 py-2 text-xs text-white placeholder-slate-500 focus:outline-none"
          />
          <div className="max-h-[220px] overflow-y-auto py-1">
            {visible.length === 0 ? (
              <p className="px-3 py-2 text-[11px] text-slate-500">No matches</p>
            ) : visible.map(opt => {
              const on = selected.includes(opt)
              return (
                <button
                  type="button"
                  key={opt}
                  onClick={() => toggle(opt)}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/[0.04] text-left"
                >
                  <span className={cn(
                    'w-3.5 h-3.5 rounded grid place-items-center border shrink-0',
                    on ? 'bg-violet-600 border-violet-600' : 'border-white/30',
                  )}>
                    {on && <span className="text-[10px] text-white leading-none">✓</span>}
                  </span>
                  {opt}
                </button>
              )
            })}
          </div>
          {addable && (
            <div className="border-t border-white/[0.08] px-2.5 py-2 flex gap-1.5">
              <input
                value={newVal}
                onChange={e => setNewVal(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleAdd() }}
                placeholder={addPlaceholder}
                className="flex-1 bg-white/[0.03] border border-white/[0.08] rounded-md px-2 py-1 text-xs text-white placeholder-slate-500 focus:outline-none"
              />
              <button
                type="button"
                onClick={handleAdd}
                className="text-[11px] px-2.5 py-1 rounded-md bg-violet-600 hover:bg-violet-500 text-white"
              >
                + Add
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
