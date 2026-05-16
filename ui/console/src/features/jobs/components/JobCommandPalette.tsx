// Placeholder for Wave 2 Agent E.
//
// Will render the Cmd+K command palette built on shadcn `Command` (cmdk).
// The palette accepts an open/onOpenChange pair so the page can drive it
// from useJobKeyboard's onOpenPalette callback. Wave 2 fleshes out:
//   - Action list (every shortcut by name)
//   - Recent-jobs jump-to entries
//   - Tab-jump entries
//   - Filter-toggle entries
//
// Returning null today keeps the JSX in JobsPage stable so the wiring
// (mount point, open state, hotkey binding) doesn't need to change when
// Wave 2 ships.

export interface JobCommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function JobCommandPalette({ open, onOpenChange }: JobCommandPaletteProps) {
  // Accessed so the placeholder satisfies the contract without unused-prop
  // warnings. Wave 2 wires both into <Command> / <CommandDialog>.
  void open
  void onOpenChange
  return null
}
