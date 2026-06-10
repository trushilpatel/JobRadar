import { useState, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils/utils'

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
  options,
  selected,
  onChange,
  placeholder = 'All',
  singularLabel = (n) => `${n} selected`,
  searchPlaceholder = 'Search…',
  addable,
  onAdd,
  addPlaceholder = 'Add new…',
  leadingIcon,
  ...rest
}: MultiSelectDropdownProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [newVal, setNewVal] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const portalRef = useRef<HTMLDivElement>(null)
  const [dropdownPos, setDropdownPos] = useState({ top: 0, left: 0, width: 0 })

  // H5: close on mousedown and touchstart outside both the trigger and the portal
  useEffect(() => {
    if (!open) return
    function handler(e: MouseEvent | TouchEvent) {
      const target = e.target as Node
      if (
        containerRef.current &&
        !containerRef.current.contains(target) &&
        (!portalRef.current || !portalRef.current.contains(target))
      ) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    document.addEventListener('touchstart', handler)
    return () => {
      document.removeEventListener('mousedown', handler)
      document.removeEventListener('touchstart', handler)
    }
  }, [open])

  useEffect(() => {
    if (!open) {
      setSearch('')
      setNewVal('')
    }
  }, [open])

  function openDropdown() {
    const rect = containerRef.current?.getBoundingClientRect()
    if (rect) {
      setDropdownPos({ top: rect.bottom + 4, left: rect.left, width: rect.width })
    }
    setOpen((o) => !o)
  }

  const visible = options
    .filter((o) => o.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => {
      const aOn = selected.includes(a)
      const bOn = selected.includes(b)
      if (aOn && !bOn) return -1
      if (!aOn && bOn) return 1
      return 0
    })
  const label =
    selected.length === 0
      ? placeholder
      : selected.length === 1
        ? selected[0]
        : singularLabel(selected.length)

  function toggle(v: string) {
    onChange(selected.includes(v) ? selected.filter((x) => x !== v) : [...selected, v])
  }

  function handleAdd() {
    const v = newVal.trim().toLowerCase()
    if (!v) return
    onAdd?.(v)
    if (!selected.includes(v)) onChange([...selected, v])
    setNewVal('')
  }

  return (
    <div className="relative" ref={containerRef}>
      <button
        type="button"
        aria-label={rest['aria-label']}
        aria-expanded={open}
        onClick={openDropdown}
        className={cn(
          'inline-flex items-center gap-1.5 text-[11px] px-2.5 py-1.5 rounded-lg border transition-colors',
          selected.length > 0
            ? 'bg-violet-500/15 border-violet-500/35 text-violet-300'
            : 'bg-subtle border-[var(--border)] text-muted hover:bg-hover'
        )}
      >
        {leadingIcon}
        <span>{label}</span>
        <ChevronDown size={10} />
      </button>

      {/* H6: render in a portal so overflow-hidden parents don't clip it */}
      {open &&
        createPortal(
          <div
            ref={portalRef}
            style={{
              position: 'fixed',
              top: dropdownPos.top,
              left: dropdownPos.left,
              width: Math.max(dropdownPos.width, 240),
              zIndex: 50,
            }}
            className="bg-panel border border-[var(--border)] rounded-[10px] shadow-[0_8px_24px_rgba(0,0,0,0.4)] overflow-hidden"
          >
            <input
              autoFocus
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={searchPlaceholder}
              className="w-full bg-subtle border-b border-[var(--border)] px-2.5 py-2 text-xs text-primary placeholder:text-placeholder focus:outline-none"
            />
            <div className="max-h-[220px] overflow-y-auto py-1">
              {visible.length === 0 ? (
                <p className="px-3 py-2 text-[11px] text-muted">No matches</p>
              ) : (
                visible.map((opt) => {
                  const on = selected.includes(opt)
                  return (
                    <button
                      type="button"
                      key={opt}
                      onClick={() => toggle(opt)}
                      className="w-full flex items-center gap-2 px-2.5 py-1.5 text-xs text-secondary hover:bg-hover text-left"
                    >
                      <span
                        className={cn(
                          'w-3.5 h-3.5 rounded grid place-items-center border shrink-0',
                          on ? 'bg-violet-600 border-violet-600' : 'border-[var(--border-strong)]'
                        )}
                      >
                        {on && <span className="text-[10px] text-white leading-none">✓</span>}
                      </span>
                      {opt}
                    </button>
                  )
                })
              )}
            </div>
            {addable && (
              <div className="border-t border-[var(--border)] px-2.5 py-2 flex gap-1.5">
                <input
                  value={newVal}
                  onChange={(e) => setNewVal(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleAdd()
                  }}
                  placeholder={addPlaceholder}
                  className="flex-1 bg-subtle border border-[var(--border)] rounded-md px-2 py-1 text-xs text-primary placeholder:text-placeholder focus:outline-none"
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
          </div>,
          document.body
        )}
    </div>
  )
}
