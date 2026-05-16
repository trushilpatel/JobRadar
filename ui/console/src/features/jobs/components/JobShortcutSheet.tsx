import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Kbd, KbdGroup } from '@/components/ui/kbd'
import { cn } from '@/lib/utils'

export interface JobShortcutSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface ShortcutRow {
  chords: string[][]
  label: string
}

const NAVIGATION_SHORTCUTS: ShortcutRow[] = [
  { chords: [['j'], ['↓']], label: 'Move focus to next row' },
  { chords: [['k'], ['↑']], label: 'Move focus to previous row' },
  { chords: [['⏎']], label: 'Open preview (desktop) / push detail route (mobile)' },
  { chords: [['⌘', '⏎']], label: 'Open posting URL in a new tab' },
  { chords: [['esc']], label: 'Close preview / clear selection / exit search' },
  { chords: [['←']], label: 'Previous page' },
  { chords: [['→']], label: 'Next page' },
  { chords: [['g', 'g']], label: 'Jump to top of list' },
  { chords: [['⇧', 'G']], label: 'Jump to bottom of list' },
]

const TAB_SHORTCUTS: ShortcutRow[] = [
  { chords: [['g', 'n']], label: 'Go to New' },
  { chords: [['g', 's']], label: 'Go to Saved' },
  { chords: [['g', 'a']], label: 'Go to Applied' },
  { chords: [['g', 'i']], label: 'Go to Interviewing' },
  { chords: [['g', 'o']], label: 'Go to Offer' },
  { chords: [['g', 'r']], label: 'Go to Rejected' },
  { chords: [['⌥', '1']], label: 'Tab 1 (New)' },
  { chords: [['⌥', '2']], label: 'Tab 2 (Saved)' },
  { chords: [['⌥', '3']], label: 'Tab 3 (Applied)' },
  { chords: [['⌥', '4']], label: 'Tab 4 (Interviewing)' },
  { chords: [['⌥', '5']], label: 'Tab 5 (Offer)' },
  { chords: [['⌥', '6']], label: 'Tab 6 (Rejected)' },
]

const STATUS_SHORTCUTS: ShortcutRow[] = [
  { chords: [['s']], label: 'Save' },
  { chords: [['a']], label: 'Applied' },
  { chords: [['i']], label: 'Interviewing' },
  { chords: [['o']], label: 'Offer' },
  { chords: [['r']], label: 'Rejected' },
  { chords: [['d']], label: 'Dismiss (opens reason modal)' },
  { chords: [['u']], label: 'Undo last status change' },
]

const SELECTION_SHORTCUTS: ShortcutRow[] = [
  { chords: [['x']], label: 'Toggle row selection' },
  { chords: [['⇧', 'X']], label: 'Range select (last selected → focused)' },
  { chords: [['⌘', 'A']], label: 'Select all on page' },
  { chords: [['⇧', 'S']], label: 'Bulk save' },
  { chords: [['⇧', 'A']], label: 'Bulk applied' },
  { chords: [['⇧', 'D']], label: 'Bulk dismiss' },
]

const UTILITY_SHORTCUTS: ShortcutRow[] = [
  { chords: [['/']], label: 'Focus search input' },
  { chords: [['⌘', 'K']], label: 'Open command palette' },
  { chords: [['⌘', '⇧', 'R']], label: 'Reset filters' },
  { chords: [['?']], label: 'Open shortcut help sheet' },
]

function ShortcutSection({
  title,
  rows,
  className,
}: {
  title: string
  rows: ShortcutRow[]
  className?: string
}) {
  return (
    <section
      className={cn('border-t border-slate-800 pt-4 first:border-t-0 first:pt-0', className)}
    >
      <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-400">{title}</h3>
      <dl className="space-y-2">
        {rows.map((row, idx) => (
          <div
            key={`${title}-${idx}`}
            className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-1"
          >
            <dt className="flex items-center gap-2">
              {row.chords.map((chord, chordIdx) => (
                <KbdGroup key={chordIdx}>
                  {chord.map((key, keyIdx) => (
                    <Kbd key={`${chordIdx}-${keyIdx}`}>{key}</Kbd>
                  ))}
                </KbdGroup>
              ))}
            </dt>
            <dd className="text-sm text-slate-300">{row.label}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

export function JobShortcutSheet({ open, onOpenChange }: JobShortcutSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>Keyboard shortcuts</SheetTitle>
          <SheetDescription>Press ? to toggle this panel.</SheetDescription>
        </SheetHeader>
        <div className="mt-6 space-y-6 px-4 pb-6">
          <ShortcutSection title="Navigation" rows={NAVIGATION_SHORTCUTS} />
          <ShortcutSection title="Tabs / views" rows={TAB_SHORTCUTS} />
          <ShortcutSection title="Status actions" rows={STATUS_SHORTCUTS} />
          <ShortcutSection title="Selection / bulk" rows={SELECTION_SHORTCUTS} />
          <ShortcutSection title="Search / palette / help" rows={UTILITY_SHORTCUTS} />
        </div>
      </SheetContent>
    </Sheet>
  )
}
