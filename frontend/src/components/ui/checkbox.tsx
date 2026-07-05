import * as React from 'react'
import * as CheckboxPrimitive from '@radix-ui/react-checkbox'
import { Check } from 'lucide-react'

import { cn } from '@/lib/utils/utils'

const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      // `violet` is a flat design token (see tailwind.config.js), not a
      // numbered shade scale — `bg-violet-600`-style classes silently fail to
      // generate. Use the bare token (it already adapts between light/dark
      // via the underlying --violet CSS var) instead of a numbered shade.
      'grid place-content-center peer h-4 w-4 shrink-0 rounded-sm border border-[var(--border-strong)] ring-offset-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-violet data-[state=checked]:bg-violet data-[state=checked]:text-white dark:ring-offset-slate-950',
      className
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator className={cn('grid place-content-center text-current')}>
      <Check className="h-4 w-4" />
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
))
Checkbox.displayName = CheckboxPrimitive.Root.displayName

export { Checkbox }
