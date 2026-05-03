import { useState, useEffect } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ADAPTER_TYPES } from '@/types'
import type { Company } from '@/types'
import { createCompany, updateCompany } from '@/api/companies'
import { ApiValidationError } from '@/lib/api-error'

interface Props {
  open: boolean
  onClose: () => void
  /** Pass a company to enter edit mode; omit for create mode. */
  initialData?: Company
  onCreated?: (company: Company) => void
  onUpdated?: (company: Company) => void
}

// URL templates for adapters that have a known API pattern.
const URL_TEMPLATES: Partial<Record<string, (id: string) => string>> = {
  greenhouse: id => `https://boards-api.greenhouse.io/v1/boards/${id}/jobs`,
  lever:      id => `https://jobs.lever.co/${id}`,
  ashby:      id => `https://jobs.ashbyhq.com/${id}`,
}

const ADAPTER_HINTS: Partial<Record<string, string>> = {
  greenhouse: 'URL auto-filled from company ID',
  lever:      'URL auto-filled from company ID',
  ashby:      'URL auto-filled from company ID',
  google:     'No URL required — uses built-in Google Careers scraper',
  apple:      'No URL required — uses built-in Apple Jobs scraper',
  meta:       'No URL required — uses built-in Meta Careers scraper',
  microsoft:  'No URL required — uses built-in Microsoft Careers scraper',
  rippling:   'No URL required — uses built-in Rippling Jobs scraper',
}

function buildUrl(adapter: string, id: string): string {
  const tmpl = URL_TEMPLATES[adapter]
  return tmpl && id ? tmpl(id) : ''
}

const FIXED_URL_ADAPTERS = new Set(['google', 'netflix', 'uber', 'apple', 'meta', 'microsoft', 'rippling'])

const EMPTY_FORM = { id: '', name: '', url: '', adapter_type: 'greenhouse', active: true }

export function AddCompanyModal({ open, onClose, initialData, onCreated, onUpdated }: Props) {
  const isEdit = !!initialData

  const [form, setForm] = useState(EMPTY_FORM)
  const [urlManuallyEdited, setUrlManuallyEdited] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [globalError, setGlobalError] = useState('')
  const [loading, setLoading] = useState(false)

  // Sync form when modal opens in edit mode
  useEffect(() => {
    if (open && initialData) {
      setForm({
        id:           initialData.id,
        name:         initialData.name,
        url:          initialData.url,
        adapter_type: initialData.adapter_type,
        active:       initialData.active,
      })
      setUrlManuallyEdited(true) // treat existing URL as manual — don't auto-overwrite
    } else if (open && !initialData) {
      setForm(EMPTY_FORM)
      setUrlManuallyEdited(false)
    }
    setFieldErrors({})
    setGlobalError('')
  }, [open, initialData])

  function handleAdapterChange(adapter: string) {
    setForm(f => ({
      ...f,
      adapter_type: adapter,
      url: !urlManuallyEdited ? buildUrl(adapter, f.id) : f.url,
    }))
    setFieldErrors({})
    setGlobalError('')
  }

  function handleIdChange(id: string) {
    setForm(f => ({
      ...f,
      id,
      url: !urlManuallyEdited ? buildUrl(f.adapter_type, id) : f.url,
    }))
    setFieldErrors({})
    setGlobalError('')
  }

  function handleUrlChange(url: string) {
    setUrlManuallyEdited(true)
    setForm(f => ({ ...f, url }))
    setFieldErrors({})
    setGlobalError('')
  }

  function handleClose() {
    setForm(EMPTY_FORM)
    setUrlManuallyEdited(false)
    setFieldErrors({})
    setGlobalError('')
    onClose()
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.id || !form.name || (!FIXED_URL_ADAPTERS.has(form.adapter_type) && !form.url)) {
      setGlobalError(
        FIXED_URL_ADAPTERS.has(form.adapter_type)
          ? 'Identifier and name are required'
          : 'Identifier, name, and URL are required'
      )
      return
    }
    setLoading(true)
    try {
      if (isEdit && initialData) {
        await updateCompany(initialData.id, {
          name:         form.name,
          url:          form.url,
          adapter_type: form.adapter_type,
          active:       form.active,
        })
        onUpdated?.({ ...initialData, ...form })
      } else {
        const company = await createCompany(form)
        onCreated?.(company)
      }
      handleClose()
    } catch (err: unknown) {
      if (err instanceof ApiValidationError) {
        setFieldErrors(err.fieldErrors())
      } else {
        setGlobalError(err instanceof Error ? err.message : `Failed to ${isEdit ? 'update' : 'create'} company`)
      }
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
              {isEdit ? 'Edit Company' : 'Add Company'}
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
              {fieldErrors.adapter_type && <p className="text-xs text-red-400 mt-1">{fieldErrors.adapter_type}</p>}
            </Field>

            <Field label="Identifier (slug)" hint={isEdit ? 'Cannot be changed' : 'Unique slug, e.g. stripe'}>
              <input
                value={form.id}
                onChange={e => !isEdit && handleIdChange(e.target.value.toLowerCase().replace(/\s+/g, '-'))}
                placeholder="stripe"
                disabled={isEdit}
                className={cn(inputClass, isEdit && 'opacity-50 cursor-not-allowed')}
              />
              {fieldErrors.id && <p className="text-xs text-red-400 mt-1">{fieldErrors.id}</p>}
            </Field>

            <Field label="Name">
              <input
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="Stripe"
                className={inputClass}
              />
              {fieldErrors.name && <p className="text-xs text-red-400 mt-1">{fieldErrors.name}</p>}
            </Field>

            <Field label="Careers URL" hint={isEdit ? undefined : urlHint}>
              <div className="relative">
                <input
                  value={form.url}
                  onChange={e => handleUrlChange(e.target.value)}
                  placeholder={hasTemplate && !isEdit ? 'Auto-filled — enter Company ID above' : 'https://...'}
                  className={cn(
                    inputClass,
                    'pr-9',
                    hasTemplate && !urlManuallyEdited && form.url && 'text-slate-400'
                  )}
                />
                <div className="absolute right-2.5 top-1/2 -translate-y-1/2 flex items-center gap-1.5">
                  {hasTemplate && urlManuallyEdited && !isEdit && (
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
              {hasTemplate && !urlManuallyEdited && form.url && !isEdit && (
                <p className="text-[11px] text-violet-400/70 mt-1">✓ Auto-filled from identifier</p>
              )}
              {fieldErrors.url && <p className="text-xs text-red-400 mt-1">{fieldErrors.url}</p>}
            </Field>

            <div className="flex items-center justify-between">
              <span className="text-sm text-slate-300">Start scanning immediately</span>
              <button
                type="button"
                role="switch"
                aria-checked={form.active}
                onClick={() => setForm(f => ({ ...f, active: !f.active }))}
                className={cn(
                  'relative inline-flex w-10 h-6 rounded-full transition-colors duration-200 focus:outline-none shrink-0',
                  form.active ? 'bg-violet-600' : 'bg-white/15'
                )}
              >
                <span className={cn(
                  'absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform duration-200',
                  form.active ? 'translate-x-5' : 'translate-x-1'
                )} />
              </button>
            </div>

            {globalError && <p className="text-sm text-red-400">{globalError}</p>}

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
                {loading ? (isEdit ? 'Saving…' : 'Adding…') : (isEdit ? 'Save Changes' : 'Add Company')}
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
