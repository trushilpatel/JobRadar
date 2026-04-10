import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium',
  {
    variants: {
      variant: {
        default:  'bg-white/[0.07] text-slate-300',
        mono:     'bg-white/[0.07] text-slate-300 font-mono',
        success:  'bg-emerald-500/15 text-emerald-400',
        error:    'bg-red-500/15 text-red-400',
        warning:  'bg-amber-500/15 text-amber-400',
        info:     'bg-blue-500/15 text-blue-400',
        violet:   'bg-violet-500/20 text-violet-300',
        ghost:    'text-slate-500',
      },
    },
    defaultVariants: { variant: 'default' },
  }
)

interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />
}
