import { useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import type { Company } from '@/types'
import {
  deleteAdminCompany,
  updateAdminCompanyScanSettings,
  setAllAdminCompaniesActive,
} from '../api/adminCompanies.api'
import { triggerScan, triggerScanAll } from '../api/companies.api'
import { companyQueryKeys } from '../queryKeys'

export function useCompanyActions() {
  const qc = useQueryClient()
  const [scanning, setScanning] = useState<Set<string>>(new Set())
  const [bulkToggling, setBulkToggling] = useState(false)
  const [runningAll, setRunningAll] = useState(false)

  function invalidate() {
    qc.invalidateQueries({ queryKey: companyQueryKeys.adminCompaniesBase() })
  }

  async function toggleActive(company: Company) {
    const next = !company.active
    try {
      await updateAdminCompanyScanSettings(company.id, { active: next })
      toast.success(`${company.name} ${next ? 'activated' : 'deactivated'}`)
      invalidate()
    } catch {
      toast.error('Failed to update company')
    }
  }

  async function remove(id: string) {
    try {
      await deleteAdminCompany(id)
      toast.success('Company deleted')
      invalidate()
    } catch {
      toast.error('Failed to delete company')
    }
  }

  async function scan(company: Company) {
    setScanning((s) => new Set(s).add(company.id))
    try {
      await triggerScan(company.id)
      toast.success(`Scan queued for ${company.name}`)
      qc.invalidateQueries({ queryKey: companyQueryKeys.scanQueue() })
      qc.invalidateQueries({ queryKey: companyQueryKeys.adminCompaniesBase() })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to queue scan')
    } finally {
      setScanning((s) => {
        const n = new Set(s)
        n.delete(company.id)
        return n
      })
    }
  }

  async function scanAll() {
    setRunningAll(true)
    try {
      const { queued } = await triggerScanAll()
      toast.success(`${queued} ${queued === 1 ? 'company' : 'companies'} queued for scan`)
      qc.invalidateQueries({ queryKey: companyQueryKeys.scanQueue() })
      qc.invalidateQueries({ queryKey: companyQueryKeys.adminCompaniesBase() })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to queue scans')
    } finally {
      setRunningAll(false)
    }
  }

  async function bulkToggleActive(currentAllActive: boolean) {
    const next = !currentAllActive
    setBulkToggling(true)
    try {
      await setAllAdminCompaniesActive(next)
      toast.success(next ? 'All companies activated' : 'All companies deactivated')
      invalidate()
    } catch {
      toast.error('Failed to update companies')
    } finally {
      setBulkToggling(false)
    }
  }

  async function handleCompanyAdded(company: Company) {
    invalidate()
    if (company.active) {
      try {
        await triggerScan(company.id)
        toast.success(`${company.name} added and scan queued`)
        invalidate()
      } catch {
        toast.success(`${company.name} added — click Scan to start scanning`)
      }
    } else {
      toast.success(`${company.name} added`)
    }
  }

  function handleCompanyUpdated(company: Company) {
    invalidate()
    toast.success(`${company.name} updated`)
  }

  return {
    scanning,
    bulkToggling,
    runningAll,
    toggleActive,
    remove,
    scan,
    scanAll,
    bulkToggleActive,
    handleCompanyAdded,
    handleCompanyUpdated,
  }
}
