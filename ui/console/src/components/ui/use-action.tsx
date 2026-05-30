import { TOKENS, type TokenKey } from '@/config/tokens'
import { useUserConfig } from '@/lib/hooks/useUserConfig'
import { Kbd } from './kbd'

export interface ActionToken {
  label: string
  shortcut: string | null
  showKbd: boolean
  kbd: React.ReactNode | null
}

export function useAction(key: TokenKey): ActionToken {
  const { data: config } = useUserConfig()
  const token = TOKENS[key]
  const shortcut = 'shortcut' in token ? (token.shortcut as string) : null
  const showKbd = !!shortcut && (config?.show_keyboard_shortcuts ?? false)
  return {
    label: token.copy,
    shortcut: shortcut ?? null,
    showKbd,
    kbd: showKbd ? <Kbd>{shortcut}</Kbd> : null,
  }
}
