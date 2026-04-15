import { useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import type { Company } from '@/types'
import {
  updateCompany, deleteCompany, triggerScan, setAllCompaniesActive,
} from '@/api/companies'

export function useCompanyActions() {
  const qc = useQueryClient()
  const [scanning, setScanning] = useState<Set<string>>(new Set())
  const [bulkToggling, setBulkToggling] = useState(false)

  function invalidate() {
    qc.invalidateQueries({ queryKey: ['companies'] })
  }

  async function toggleActive(company: Company) {
    const next = !company.active
    try {
      await updateCompany(company.id, { active: next })
      invalidate()
    } catch {
      toast.error('Failed to update company')
    }
  }

  async function remove(id: string) {
    if (!confirm('Delete this company? This cannot be undone.')) return
    try {
      await deleteCompany(id)
      toast.success('Company deleted')
      invalidate()
    } catch {
      toast.error('Failed to delete company')
    }
  }

  async function scan(company: Company) {
    setScanning(s => new Set(s).add(company.id))
    try {
      await triggerScan(company.id)
      toast.success(`Scan queued for ${company.name}`)
      qc.invalidateQueries({ queryKey: ['companies', 'queue'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to queue scan')
    } finally {
      setScanning(s => { const n = new Set(s); n.delete(company.id); return n })
    }
  }

  async function bulkToggleActive(currentAllActive: boolean) {
    const next = !currentAllActive
    setBulkToggling(true)
    try {
      await setAllCompaniesActive(next)
      toast.success(next ? 'All companies activated' : 'All companies deactivated')
      invalidate()
    } catch {
      toast.error('Failed to update companies')
    } finally {
      setBulkToggling(false)
    }
  }

  return {
    scanning,
    bulkToggling,
    toggleActive,
    remove,
    scan,
    bulkToggleActive,
  }
}
