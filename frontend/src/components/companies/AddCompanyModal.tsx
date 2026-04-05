import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ADAPTER_TYPES } from '@/types'
import type { Company } from '@/types'
import { createCompany } from '@/api/companies'

interface Props {
  open: boolean
  onClose: () => void
  onCreated: (company: Company) => void
}

// URL templates for adapters that have a known API pattern.
// {id} is replaced with the company ID the user types.
const URL_TEMPLATES: Partial<Record<string, (id: string) => string>> = {
  greenhouse: id => `https://boards-api.greenhouse.io/v1/boards/${id}/jobs`,
  lever:      id => `https://api.lever.co/v0/postings/${id}`,
  ashby:      id => `https://api.ashbyhq.com/posting-api/job-board/${id}`,
  workable:   id => `https://${id}.workable.com/spi/v3/jobs`,
}

const ADAPTER_HINTS: Partial<Record<string, string>> = {
  greenhouse: 'URL auto-filled from company ID',
  lever:      'URL auto-filled from company ID',
  ashby:      'URL auto-filled from company ID',
  workable:   'URL auto-filled from company ID',
  workday:    'Paste the full Workday API endpoint',
  chromedp:   'Paste the careers page URL (browser-rendered)',
}

function buildUrl(adapter: string, id: string): string {
  const tmpl = URL_TEMPLATES[adapter]
  return tmpl && id ? tmpl(id) : ''
}

export function AddCompanyModal({ open, onClose, onCreated }: Props) {
  const [form, setForm] = useState({
    id: '',
    name: '',
    url: '',
    adapter_type: 'greenhouse',
    active: true,
  })
  // Track whether the user has manually edited the URL so we stop overwriting it
  const [urlManuallyEdited, setUrlManuallyEdited] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  function handleAdapterChange(adapter: string) {
    setForm(f => {
      const newUrl = !urlManuallyEdited ? buildUrl(adapter, f.id) : f.url
      return { ...f, adapter_type: adapter, url: newUrl }
    })
    setError('')
  }

  function handleIdChange(id: string) {
    setForm(f => {
      const newUrl = !urlManuallyEdited ? buildUrl(f.adapter_type, id) : f.url
      return { ...f, id, url: newUrl }
    })
    setError('')
  }

  function handleUrlChange(url: string) {
    setUrlManuallyEdited(true)
    setForm(f => ({ ...f, url }))
    setError('')
  }

  function handleClose() {
    setForm({ id: '', name: '', url: '', adapter_type: 'greenhouse', active: true })
    setUrlManuallyEdited(false)
    setError('')
    onClose()
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
      handleClose()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to create company')
    } finally {
      setLoading(false)
    }
  }

  const urlHint = ADAPTER_HINTS[form.adapter_type]
  const hasTemplate = !!URL_TEMPLATES[form.adapter_type]

  return (
    <Dialog.Root open={open} onOpenChange={v => !v && handleClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-[#13131a] border border-white/10 rounded-xl shadow-2xl p-6">
          <div className="flex items-center justify-between mb-6">
            <Dialog.Title className="text-lg font-semibold text-white">
              Add Company
            </Dialog.Title>
            <button onClick={handleClose} className="text-slate-400 hover:text-white transition-colors">
              <X size={18} />
            </button>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Adapter first — drives URL template */}
            <Field label="Adapter">
              <select
                value={form.adapter_type}
                onChange={e => handleAdapterChange(e.target.value)}
                className={inputClass}
              >
                {ADAPTER_TYPES.map(a => (
                  <option key={a} value={a}>{a}</option>
                ))}
              </select>
            </Field>

            <Field label="Company ID" hint="Unique slug, e.g. stripe">
              <input
                value={form.id}
                onChange={e => handleIdChange(e.target.value.toLowerCase().replace(/\s+/g, '-'))}
                placeholder="stripe"
                className={inputClass}
              />
            </Field>

            <Field label="Name">
              <input
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="Stripe"
                className={inputClass}
              />
            </Field>

            <Field
              label="Careers URL"
              hint={urlHint}
            >
              <div className="relative">
                <input
                  value={form.url}
                  onChange={e => handleUrlChange(e.target.value)}
                  placeholder={
                    hasTemplate
                      ? `Auto-filled — enter Company ID above`
                      : 'https://...'
                  }
                  className={cn(
                    inputClass,
                    'pr-9',
                    hasTemplate && !urlManuallyEdited && form.url && 'text-slate-400'
                  )}
                />
                <div className="absolute right-2.5 top-1/2 -translate-y-1/2 flex items-center gap-1.5">
                  {hasTemplate && urlManuallyEdited && (
                    <button
                      type="button"
                      onClick={() => {
                        setUrlManuallyEdited(false)
                        setForm(f => ({ ...f, url: buildUrl(f.adapter_type, f.id) }))
                      }}
                      className="text-xs text-violet-400 hover:text-violet-300 transition-colors"
                    >
                      Reset
                    </button>
                  )}
                  {form.url && (
                    <a
                      href={form.url}
                      target="_blank"
                      rel="noreferrer"
                      tabIndex={-1}
                      title="Open URL"
                      className="text-slate-500 hover:text-violet-400 transition-colors"
                      onClick={e => e.stopPropagation()}
                    >
                      <ExternalLink size={13} />
                    </a>
                  )}
                </div>
              </div>
            </Field>

            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="active"
                checked={form.active}
                onChange={e => setForm(f => ({ ...f, active: e.target.checked }))}
                className="w-4 h-4 accent-violet-500"
              />
              <label htmlFor="active" className="text-sm text-slate-300">Active (start scanning)</label>
            </div>

            {error && <p className="text-sm text-red-400">{error}</p>}

            <div className="flex gap-3 pt-2">
              <button
                type="button"
                onClick={handleClose}
                className={cn(btnBase, 'flex-1 bg-white/5 hover:bg-white/10 text-slate-300')}
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={loading}
                className={cn(btnBase, 'flex-1 bg-violet-600 hover:bg-violet-500 text-white disabled:opacity-50')}
              >
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
