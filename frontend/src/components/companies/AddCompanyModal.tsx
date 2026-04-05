import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ADAPTER_TYPES } from '@/types'
import type { Company } from '@/types'
import { createCompany } from '@/api/companies'

interface Props {
  open: boolean
  onClose: () => void
  onCreated: (company: Company) => void
}

export function AddCompanyModal({ open, onClose, onCreated }: Props) {
  const [form, setForm] = useState({
    id: '',
    name: '',
    url: '',
    adapter_type: 'greenhouse',
    active: true,
  })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  function set(field: string, value: string | boolean) {
    setForm(f => ({ ...f, [field]: value }))
    setError('')
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.id || !form.name || !form.url) {
      setError('ID, name, and URL are required')
      return
    }
    setLoading(true)
    try {
      const company = await createCompany(form)
      onCreated(company)
      onClose()
      setForm({ id: '', name: '', url: '', adapter_type: 'greenhouse', active: true })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={v => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-[#13131a] border border-white/10 rounded-xl shadow-2xl p-6">
          <div className="flex items-center justify-between mb-6">
            <Dialog.Title className="text-lg font-semibold text-white">
              Add Company
            </Dialog.Title>
            <button onClick={onClose} className="text-slate-400 hover:text-white transition-colors">
              <X size={18} />
            </button>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <Field label="Company ID" hint="Unique slug, e.g. stripe">
              <input
                value={form.id}
                onChange={e => set('id', e.target.value)}
                placeholder="stripe"
                className={inputClass}
              />
            </Field>

            <Field label="Name">
              <input
                value={form.name}
                onChange={e => set('name', e.target.value)}
                placeholder="Stripe"
                className={inputClass}
              />
            </Field>

            <Field label="Careers URL">
              <input
                value={form.url}
                onChange={e => set('url', e.target.value)}
                placeholder="https://boards.greenhouse.io/stripe"
                className={inputClass}
              />
            </Field>

            <Field label="Adapter">
              <select
                value={form.adapter_type}
                onChange={e => set('adapter_type', e.target.value)}
                className={inputClass}
              >
                {ADAPTER_TYPES.map(a => (
                  <option key={a} value={a}>{a}</option>
                ))}
              </select>
            </Field>

            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="active"
                checked={form.active}
                onChange={e => set('active', e.target.checked)}
                className="w-4 h-4 accent-violet-500"
              />
              <label htmlFor="active" className="text-sm text-slate-300">Active (start scanning)</label>
            </div>

            {error && <p className="text-sm text-red-400">{error}</p>}

            <div className="flex gap-3 pt-2">
              <button type="button" onClick={onClose} className={cn(btnBase, 'flex-1 bg-white/5 hover:bg-white/10 text-slate-300')}>
                Cancel
              </button>
              <button type="submit" disabled={loading} className={cn(btnBase, 'flex-1 bg-violet-600 hover:bg-violet-500 text-white disabled:opacity-50')}>
                {loading ? 'Adding…' : 'Add Company'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-sm font-medium text-slate-300">
        {label}
        {hint && <span className="ml-2 text-xs text-slate-500">{hint}</span>}
      </label>
      {children}
    </div>
  )
}

const inputClass = 'w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-violet-500 transition-colors'
const btnBase = 'px-4 py-2 rounded-lg text-sm font-medium transition-colors cursor-pointer disabled:cursor-not-allowed'
