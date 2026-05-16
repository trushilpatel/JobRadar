// Placeholder for Wave 2 Agent C.
//
// Will render the mobile (<640px) filter drawer built on shadcn `Drawer`
// (vaul). It hosts the same controls JobFilters already renders inline
// on desktop — companies multi-select, location, exp ranges, since
// date filter — plus a reset button.
//
// Returning null today keeps the JSX in JobsPage stable. The mount
// point and the open/onOpenChange contract are set up so Wave 2 only
// edits this file.

export interface MobileFilterDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function MobileFilterDrawer({ open, onOpenChange }: MobileFilterDrawerProps) {
  void open
  void onOpenChange
  return null
}
