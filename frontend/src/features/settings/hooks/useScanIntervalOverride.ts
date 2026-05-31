import { useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { updateScanIntervalOverride } from '../api/adapterConfigs.api'
import { settingsQueryKeys, scanIntervalOverrideMutationInvalidationRoots } from '../queryKeys'

export function useScanIntervalOverride(companyId: string) {
  const qc = useQueryClient()
  const [loading, setLoading] = useState(false)

  async function setOverride(override: number | null): Promise<void> {
    setLoading(true)
    try {
      await updateScanIntervalOverride(companyId, override)
      toast.success(override === null ? 'Override cleared' : 'Scan interval override saved')
      qc.invalidateQueries({
        queryKey: scanIntervalOverrideMutationInvalidationRoots.adminCompanies,
      })
      qc.invalidateQueries({ queryKey: settingsQueryKeys.companyScanConfig(companyId) })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update override')
      throw err
    } finally {
      setLoading(false)
    }
  }

  return { setOverride, loading }
}
