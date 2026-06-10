import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils/utils'

const badgeVariants = cva(
  'inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium',
  {
    variants: {
      variant: {
        default: 'bg-subtle text-secondary',
        mono: 'bg-subtle text-secondary font-mono',
        success: 'bg-emerald-500/15 text-emerald-400',
        error: 'bg-red-500/15 text-red-400',
        warning: 'bg-amber-500/15 text-amber-400',
        info: 'bg-blue-500/15 text-blue-400',
        violet: 'bg-violet-500/20 text-violet-300',
        ghost: 'text-muted',
      },
    },
    defaultVariants: { variant: 'default' },
  }
)

interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />
}
