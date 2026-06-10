import { cn, avatarColor } from '@/lib/utils/utils'

interface CompanyAvatarProps {
  name: string
  size?: 'sm' | 'md'
}

export function CompanyAvatar({ name, size = 'sm' }: CompanyAvatarProps) {
  return (
    <div
      role="img"
      aria-label={name}
      className={cn(
        'rounded-md flex items-center justify-center font-semibold text-white shrink-0',
        avatarColor(name),
        size === 'sm' ? 'w-7 h-7 text-xs' : 'w-9 h-9 text-sm'
      )}
    >
      {name ? name[0]?.toUpperCase() : '?'}
    </div>
  )
}
