import { NAV_TOKENS } from './nav.tokens'
import { JOBS_TOKENS } from './jobs.tokens'

export const TOKENS = {
  ...NAV_TOKENS,
  ...JOBS_TOKENS,
} as const

export type TokenKey = keyof typeof TOKENS

type ActionToken = { copy: string; shortcut: string }

export function getActionTokens(): Array<{ key: TokenKey; copy: string; shortcut: string }> {
  return (Object.entries(TOKENS) as Array<[TokenKey, { copy: string; shortcut?: string }]>)
    .filter(
      (entry): entry is [TokenKey, ActionToken] =>
        'shortcut' in entry[1] && entry[1].shortcut !== undefined
    )
    .map(([key, token]) => ({ key, copy: token.copy, shortcut: token.shortcut }))
}
