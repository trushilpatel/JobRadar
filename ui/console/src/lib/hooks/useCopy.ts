import { TOKENS, type TokenKey } from '@/config/tokens'

export function useCopy(key: TokenKey): string {
  return TOKENS[key].copy
}
