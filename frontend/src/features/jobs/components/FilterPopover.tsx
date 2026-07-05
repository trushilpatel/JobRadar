import { useRef } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Checkbox } from '@/components/ui/checkbox'

export interface FilterOption {
  value: string
  label: string
}

export interface FilterPopoverProps {
  label: string
  options: FilterOption[]
  selected: string[]
  onChange: (next: string[]) => void
  /** Renders a `CommandInput` search box when true. Default false. */
  searchable?: boolean
  /** Text shown when a search matches no options. Default 'No results'. */
  emptyLabel?: string
  /** Appends ` (n)` to the trigger label when `selected.length > 0`. Default false. */
  showCount?: boolean
  className?: string
}

/**
 * Generic multi-select facet popover — the canonical "shadcn Popover +
 * Command + Checkbox" pattern (mirrors `components/ui/multi-select.tsx`),
 * but carries filter-facet semantics (label/searchable/showCount), which is
 * why it lives feature-local rather than in `components/ui/`.
 */
export function FilterPopover({
  label,
  options,
  selected,
  onChange,
  searchable = false,
  emptyLabel = 'No results',
  showCount = false,
  className,
}: FilterPopoverProps) {
  const listRef = useRef<HTMLDivElement>(null)

  function toggle(value: string) {
    if (selected.includes(value)) {
      onChange(selected.filter((v) => v !== value))
    } else {
      onChange([...selected, value])
    }
  }

  const triggerLabel = showCount && selected.length > 0 ? `${label} (${selected.length})` : label

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className={cn(
            'inline-flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-lg border transition-colors',
            // `violet` is a flat token (see tailwind.config.js), not a numbered
            // shade scale — `bg-violet-100`-style classes silently fail to
            // generate. color-mix() against the bare token is this project's
            // established pattern for a tinted background (see jobStatusConfig.ts).
            selected.length > 0
              ? 'bg-[color-mix(in_srgb,var(--violet)_14%,transparent)] border-[color-mix(in_srgb,var(--violet)_40%,transparent)] text-violet'
              : 'bg-[var(--bg-panel)] border-[var(--border-strong)] text-muted hover:bg-hover hover:border-[var(--border-strong)] hover:text-secondary',
            className
          )}
        >
          <span>{triggerLabel}</span>
          <ChevronDown size={12} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[240px] p-0 bg-panel border-[var(--border)] shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
        align="start"
        onOpenAutoFocus={(event) => {
          // Without a CommandInput, Radix has no tabbable descendant to
          // auto-focus and would focus the content container instead — that
          // sits ABOVE the cmdk root in the DOM, so its keydown handler never
          // sees bubbled key events. Focus the (tabIndex=-1, programmatically
          // focusable) CommandList directly so ArrowUp/ArrowDown/Enter reach
          // cmdk's built-in keyboard navigation.
          if (!searchable) {
            event.preventDefault()
            listRef.current?.focus()
          }
        }}
      >
        <Command>
          {searchable && <CommandInput placeholder="Search…" />}
          <CommandList ref={listRef}>
            <CommandEmpty>{emptyLabel}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => {
                const isSelected = selected.includes(option.value)
                return (
                  <CommandItem
                    key={option.value}
                    value={option.label}
                    onSelect={() => toggle(option.value)}
                    className="flex items-center gap-2 cursor-pointer"
                  >
                    <Checkbox checked={isSelected} tabIndex={-1} aria-hidden="true" />
                    <span className="text-xs text-secondary">{option.label}</span>
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
