// Placeholder for Wave 2 Agent F.
//
// Will render the `?` help overlay built on shadcn `Sheet`, showing the
// full keyboard map grouped by category (Navigation, Tabs, Status,
// Selection, Search/Palette). Wave 2 ships:
//   - Group headings + Kbd badges via the shadcn `kbd` primitive
//   - Reads the canonical map from a shared constant so the sheet stays
//     in sync with useJobKeyboard
//   - Esc + close-icon to dismiss
//
// Returning null today keeps the JSX in JobsPage stable.

export interface JobShortcutSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function JobShortcutSheet({ open, onOpenChange }: JobShortcutSheetProps) {
  void open
  void onOpenChange
  return null
}
