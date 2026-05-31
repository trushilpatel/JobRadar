import { COPY, type CopyKey } from '@/config/copy'

export function useCopy(key: CopyKey): string {
  return COPY[key]
}
