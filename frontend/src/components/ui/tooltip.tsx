import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { cn } from '@/lib/utils/utils'

/* eslint-disable react-refresh/only-export-components */
export const TooltipProvider = TooltipPrimitive.Provider
export const Tooltip = TooltipPrimitive.Root
export const TooltipTrigger = TooltipPrimitive.Trigger
/* eslint-enable react-refresh/only-export-components */

export function TooltipContent({
  className,
  sideOffset = 6,
  ...props
}: React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        sideOffset={sideOffset}
        className={cn(
          'z-50 rounded-lg border border-white/[0.08] bg-slate-900/95 px-3 py-2 text-xs text-slate-200 shadow-xl backdrop-blur',
          'data-[state=delayed-open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:fade-in-0',
          className
        )}
        {...props}
      />
    </TooltipPrimitive.Portal>
  )
}
