import * as SwitchPrimitive from '@radix-ui/react-switch'
import { cn } from '@/lib/utils/utils'

interface SwitchProps extends React.ComponentProps<typeof SwitchPrimitive.Root> {
  id?: string
}

export function Switch({ className, ...props }: SwitchProps) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        'peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--bg-page)]',
        'disabled:cursor-not-allowed disabled:opacity-50',
        // `violet` is a flat design token (see tailwind.config.js), not a
        // numbered shade scale — `bg-violet-600`-style classes silently fail
        // to generate. Bare tokens adapt between light/dark automatically;
        // the unchecked track previously used a hardcoded `bg-slate-700`,
        // which looked identically dark in both themes instead of adapting.
        'data-[state=checked]:bg-violet data-[state=checked]:border-violet',
        'data-[state=unchecked]:bg-[var(--bg-subtle)] data-[state=unchecked]:border-[var(--border-strong)]',
        className
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className={cn(
          'pointer-events-none block h-4 w-4 rounded-full bg-white shadow-md ring-0 transition-transform',
          'data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0'
        )}
      />
    </SwitchPrimitive.Root>
  )
}
