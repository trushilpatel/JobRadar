import { cn } from '@/lib/utils'

interface LogoProps {
  className?: string
}

interface LogoLockupProps extends LogoProps {
  size?: 'sm' | 'md' | 'lg'
}

const lockupSizes: Record<NonNullable<LogoLockupProps['size']>, { mark: string; text: string; gap: string }> = {
  sm: { mark: 'h-4 w-4', text: 'text-sm', gap: 'gap-1.5' },
  md: { mark: 'h-5 w-5', text: 'text-base', gap: 'gap-2' },
  lg: { mark: 'h-7 w-7', text: 'text-2xl', gap: 'gap-2.5' },
}

/**
 * LogoMark — radar perimeter, inner ring, sweep line, detected job dot.
 * Uses currentColor so it inherits text color from the surrounding context.
 */
export function LogoMark({ className }: LogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 48 48"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={cn('h-6 w-6', className)}
      role="img"
      aria-label="JobRadar"
    >
      <title>JobRadar</title>
      <circle cx="24" cy="24" r="20" strokeWidth="1.75" />
      <circle cx="24" cy="24" r="11" strokeWidth="1.75" opacity="0.45" />
      <line x1="24" y1="24" x2="38" y2="14" strokeWidth="1.75" />
      <circle cx="38" cy="14" r="3" fill="currentColor" stroke="none" />
    </svg>
  )
}

/**
 * LogoSweepingMark — animated radar sweep for loading / scanning states.
 * Respects prefers-reduced-motion.
 */
export function LogoSweepingMark({ className }: LogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 48 48"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={cn('h-6 w-6', className)}
      role="img"
      aria-label="JobRadar scanning"
    >
      <title>JobRadar scanning</title>
      <circle cx="24" cy="24" r="20" strokeWidth="1.75" />
      <circle cx="24" cy="24" r="11" strokeWidth="1.75" opacity="0.45" />
      <g
        className="motion-safe:animate-spin"
        style={{ transformOrigin: '24px 24px', transformBox: 'fill-box', animationDuration: '3.6s' }}
      >
        <line x1="24" y1="24" x2="38" y2="14" strokeWidth="1.75" />
        <circle cx="38" cy="14" r="3" fill="currentColor" stroke="none" />
      </g>
    </svg>
  )
}

/**
 * LogoLockup — mark + wordmark inline, single row, currentColor throughout.
 * Two-weight wordmark: "Job" semibold, "Radar" regular at reduced opacity.
 */
export function LogoLockup({ className, size = 'md' }: LogoLockupProps) {
  const s = lockupSizes[size]
  return (
    <span className={cn('inline-flex items-center text-current', s.gap, className)}>
      <LogoMark className={s.mark} />
      <span className={cn('font-semibold tracking-tight leading-none', s.text)}>
        Job<span className="font-normal opacity-70">Radar</span>
      </span>
    </span>
  )
}
