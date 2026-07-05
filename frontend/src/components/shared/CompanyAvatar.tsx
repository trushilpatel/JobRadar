import { cn } from '@/lib/utils/utils'
import { adapterGradient } from '@/lib/utils/adapterTokens'

interface CompanyAvatarProps {
  name: string
  adapterType?: string
  size?: 'sm' | 'md'
}

export function CompanyAvatar({ name, adapterType, size = 'sm' }: CompanyAvatarProps) {
  const initials = (() => {
    if (!name) return '?'
    const words = name.trim().split(/\s+/).filter(Boolean)
    if (words.length >= 2) {
      return words
        .slice(0, 2)
        .map((w) => w[0]?.toUpperCase() ?? '')
        .join('')
    }
    return (words[0] ?? '').slice(0, 2).toUpperCase()
  })()

  return (
    <div
      role="img"
      aria-label={name}
      className={cn(
        'rounded-md flex items-center justify-center font-semibold text-white shrink-0',
        size === 'sm' ? 'w-7 h-7 text-xs' : 'w-9 h-9 text-sm'
      )}
      style={{ background: adapterGradient(adapterType) }}
    >
      {initials}
    </div>
  )
}
