import * as React from 'react'
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

export interface MultiSelectProps {
  options: string[]
  selected: string[]
  onChange: (values: string[]) => void
  placeholder?: string
  className?: string
}

export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder = 'Select…',
  className,
}: MultiSelectProps) {
  function toggle(value: string) {
    if (selected.includes(value)) {
      onChange(selected.filter((v) => v !== value))
    } else {
      onChange([...selected, value])
    }
  }

  const triggerLabel =
    selected.length === 0
      ? placeholder
      : selected.length === 1
        ? selected[0]
        : `${selected.length} selected`

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={triggerLabel}
          className={cn(
            'inline-flex items-center gap-1.5 text-[11px] px-2.5 py-1.5 rounded-lg border transition-colors',
            selected.length > 0
              ? 'bg-violet-500/15 border-violet-500/35 text-violet-300'
              : 'bg-subtle border-[var(--border)] text-muted hover:bg-hover',
            className
          )}
        >
          <span>{triggerLabel}</span>
          <ChevronDown size={10} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[240px] p-0 bg-panel border-[var(--border)] shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
        align="start"
      >
        <Command>
          <CommandInput placeholder="Search…" />
          <CommandList>
            <CommandEmpty>No results</CommandEmpty>
            <CommandGroup>
              {options.map((option) => {
                const isSelected = selected.includes(option)
                return (
                  <CommandItem
                    key={option}
                    value={option}
                    onSelect={() => toggle(option)}
                    className="flex items-center gap-2 cursor-pointer"
                  >
                    <Checkbox
                      checked={isSelected}
                      className={cn(
                        'border shrink-0',
                        isSelected
                          ? 'bg-violet-600 border-violet-600 data-[state=checked]:bg-violet-600 data-[state=checked]:text-white'
                          : 'border-[var(--border-strong)]'
                      )}
                      tabIndex={-1}
                      aria-hidden="true"
                    />
                    <span className="text-xs text-secondary">{option}</span>
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
