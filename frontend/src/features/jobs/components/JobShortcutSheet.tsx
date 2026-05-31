import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Kbd, KbdGroup } from '@/components/ui/kbd'
import { cn } from '@/lib/utils/utils'
import { formatKeys } from '@/lib/commands/formatKeys'
import type { CommandCategory, KeySpec } from '@/lib/commands/types'
import { JOB_COMMANDS, type JobCommandId } from '../commands/jobCommands'

export interface JobShortcutSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Category heading + render order — empty categories render no heading. */
const CATEGORY_ORDER: { category: CommandCategory; title: string }[] = [
  { category: 'navigation', title: 'Navigation' },
  { category: 'tabs', title: 'Tabs' },
  { category: 'status', title: 'Status' },
  { category: 'search', title: 'Search' },
]

/** Renders a command's bindings (all aliases) as Kbd glyph groups. */
function renderKeys(keys: readonly KeySpec[]) {
  if (keys.length === 0) {
    return <span className="text-xs text-slate-600">—</span>
  }
  return keys.map((spec, ai) => (
    <span key={ai} className="flex items-center gap-1">
      {ai > 0 && <span className="text-xs text-slate-600">/</span>}
      {formatKeys(spec).map((group, gi) => (
        <KbdGroup key={gi}>
          {group.map((glyph, ki) => (
            <Kbd key={`${gi}-${ki}`}>{glyph}</Kbd>
          ))}
        </KbdGroup>
      ))}
    </span>
  ))
}

export function JobShortcutSheet({ open, onOpenChange }: JobShortcutSheetProps) {
  const allIds = Object.keys(JOB_COMMANDS) as JobCommandId[]
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>Keyboard shortcuts</SheetTitle>
          <SheetDescription>Press ? to toggle this panel.</SheetDescription>
        </SheetHeader>
        <div className="mt-6 space-y-6 px-4 pb-6">
          {CATEGORY_ORDER.map(({ category, title }) => {
            const ids = allIds.filter((id) => JOB_COMMANDS[id].category === category)
            if (ids.length === 0) return null
            return (
              <section
                key={category}
                className={cn('border-t border-slate-800 pt-4 first:border-t-0 first:pt-0')}
              >
                <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-400">
                  {title}
                </h3>
                <dl className="space-y-2">
                  {ids.map((id) => (
                    <div
                      key={id}
                      className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-1"
                    >
                      <dt className="flex items-center gap-2">
                        {renderKeys(JOB_COMMANDS[id].keys)}
                      </dt>
                      <dd className="text-sm text-slate-300">{JOB_COMMANDS[id].label}</dd>
                    </div>
                  ))}
                </dl>
              </section>
            )
          })}
        </div>
      </SheetContent>
    </Sheet>
  )
}
