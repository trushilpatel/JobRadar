import { useState, useEffect } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import type { Company, ATS } from '@/types'
import {
  createAdminCompany,
  updateAdminCompany,
  updateAdminCompanyScanSettings,
} from '../api/adminCompanies.api'
import { ApiValidationError } from '@/lib/core/api-error'

interface Props {
  open: boolean
  onClose: () => void
  /** Pass a company to enter edit mode; omit for create mode. */
  initialData?: Company
  onCreated?: (company: Company) => void
  onUpdated?: (company: Company) => void
  /** ATS list from the parent (fetched at page level). Defaults to [] while loading. */
  atsList?: ATS[]
}

const ADAPTER_HINTS: Partial<Record<string, string>> = {
  greenhouse: 'URL auto-filled from company ID',
  lever: 'URL auto-filled from company ID',
  ashby: 'URL auto-filled from company ID',
  gem: 'URL auto-filled from company ID',
  deel: 'URL auto-filled from company ID',
  google: 'No URL required — uses built-in Google Careers scraper',
  apple: 'No URL required — uses built-in Apple Jobs scraper',
  meta: 'No URL required — uses built-in Meta Careers scraper',
  microsoft: 'No URL required — uses built-in Microsoft Careers scraper',
  rippling: 'No URL required — uses built-in Rippling Jobs scraper',
  workday: 'URL auto-filled from Workday host + site',
}

function buildUrl(ats: ATS | undefined, id: string): string {
  if (!ats?.base_url) return ''
  return ats.base_url + id
}

const ADAPTER_WORKDAY = 'workday'
const WORKDAY_HOST_REGEX = /^[a-z0-9-]+\.wd[0-9]+\.myworkdayjobs\.com$/

function buildWorkdayUrl(host: string, site: string): string {
  return host && site ? `https://${host}/${site}` : ''
}

function validateWorkdayHost(host: string): string {
  if (!host) return 'Workday host is required'
  if (!WORKDAY_HOST_REGEX.test(host)) {
    return 'Host must match <tenant>.wd<N>.myworkdayjobs.com (lowercase)'
  }
  return ''
}

function validateWorkdaySite(site: string): string {
  if (!site) return 'Site path is required'
  if (/\s/.test(site)) return 'Site path must not contain whitespace'
  if (site.includes('/')) return 'Site path must not contain "/"'
  return ''
}

// Scan-interval override is optional; blank means "use the global default".
// When provided it must be a positive integer number of minutes.
function validateIntervalOverride(raw: string): string {
  const value = raw.trim()
  if (value === '') return ''
  if (!/^\d+$/.test(value) || Number(value) <= 0) {
    return 'Must be a positive whole number of minutes'
  }
  return ''
}

const EMPTY_FORM = { id: '', name: '', website: '', adapter_type: 'greenhouse', active: true }
const EMPTY_WORKDAY = { host: '', site: '' }

// Payload shape we send to createAdminCompany / updateAdminCompanyScanSettings.
// adapter_config is part of the backend contract but not declared on
// createAdminCompany's parameter type, so we widen at the call site only here.
type CreateScanSettings = {
  adapter_type: string
  active: boolean
  adapter_config?: Record<string, unknown>
}

export function AddCompanyModal({
  open,
  onClose,
  initialData,
  onCreated,
  onUpdated,
  atsList = [],
}: Props) {
  const isEdit = !!initialData

  const [form, setForm] = useState(EMPTY_FORM)
  const [workday, setWorkday] = useState(EMPTY_WORKDAY)
  // Per-company scan-interval override (minutes). Edit-only; '' means "use the
  // global default". Kept as a string so the input stays controlled and blank.
  const [intervalOverride, setIntervalOverride] = useState('')
  const [urlManuallyEdited, setUrlManuallyEdited] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [globalError, setGlobalError] = useState('')
  const [loading, setLoading] = useState(false)

  const isWorkday = form.adapter_type === ADAPTER_WORKDAY
  const currentAts = atsList.find((a) => a.name === form.adapter_type)

  // Sync form when modal opens in edit mode
  useEffect(() => {
    if (open && initialData) {
      setForm({
        id: initialData.id,
        name: initialData.name,
        website: initialData.url,
        adapter_type: initialData.adapter_type,
        active: initialData.active,
      })
      setWorkday(EMPTY_WORKDAY)
      const override = initialData.scan_settings?.scan_interval_override
      setIntervalOverride(override != null ? String(override) : '')
      setUrlManuallyEdited(true) // treat existing URL as manual — don't auto-overwrite
    } else if (open && !initialData) {
      setForm(EMPTY_FORM)
      setWorkday(EMPTY_WORKDAY)
      setIntervalOverride('')
      setUrlManuallyEdited(false)
    }
    setFieldErrors({})
    setGlobalError('')
  }, [open, initialData])

  function handleAdapterChange(adapter: string) {
    const nextAts = atsList.find((a) => a.name === adapter)
    setForm((f) => {
      if (adapter === ADAPTER_WORKDAY) {
        // Workday derives the URL from host + site rather than the slug.
        return {
          ...f,
          adapter_type: adapter,
          website: buildWorkdayUrl(workday.host, workday.site),
        }
      }
      return {
        ...f,
        adapter_type: adapter,
        website: !urlManuallyEdited ? buildUrl(nextAts, f.id) : f.website,
      }
    })
    setFieldErrors({})
    setGlobalError('')
  }

  function handleIdChange(id: string) {
    setForm((f) => ({
      ...f,
      id,
      website:
        f.adapter_type === ADAPTER_WORKDAY
          ? f.website
          : !urlManuallyEdited
            ? buildUrl(
                atsList.find((a) => a.name === f.adapter_type),
                id
              )
            : f.website,
    }))
    setFieldErrors({})
    setGlobalError('')
  }

  function handleUrlChange(url: string) {
    setUrlManuallyEdited(true)
    setForm((f) => ({ ...f, website: url }))
    setFieldErrors({})
    setGlobalError('')
  }

  function handleWorkdayHostChange(host: string) {
    const normalized = host.trim()
    const next = { ...workday, host: normalized }
    setWorkday(next)
    setForm((f) => ({ ...f, website: buildWorkdayUrl(next.host, next.site) }))
    setFieldErrors((errs) => {
      const updated = { ...errs }
      const msg = validateWorkdayHost(normalized)
      if (msg) updated.workday_host = msg
      else delete updated.workday_host
      return updated
    })
    setGlobalError('')
  }

  function handleWorkdaySiteChange(site: string) {
    const normalized = site.trim()
    const next = { ...workday, site: normalized }
    setWorkday(next)
    setForm((f) => ({ ...f, website: buildWorkdayUrl(next.host, next.site) }))
    setFieldErrors((errs) => {
      const updated = { ...errs }
      const msg = validateWorkdaySite(normalized)
      if (msg) updated.workday_site = msg
      else delete updated.workday_site
      return updated
    })
    setGlobalError('')
  }

  function handleClose() {
    setForm(EMPTY_FORM)
    setWorkday(EMPTY_WORKDAY)
    setIntervalOverride('')
    setUrlManuallyEdited(false)
    setFieldErrors({})
    setGlobalError('')
    onClose()
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    // Interval override is edit-only; validate before any network call so a bad
    // value blocks submit with an inline field error.
    const intervalErr = isEdit ? validateIntervalOverride(intervalOverride) : ''
    if (intervalErr) {
      setFieldErrors((errs) => ({ ...errs, interval_override: intervalErr }))
      return
    }
    if (isWorkday) {
      const hostErr = validateWorkdayHost(workday.host)
      const siteErr = validateWorkdaySite(workday.site)
      if (hostErr || siteErr || !form.id || !form.name) {
        const errs: Record<string, string> = {}
        if (hostErr) errs.workday_host = hostErr
        if (siteErr) errs.workday_site = siteErr
        setFieldErrors(errs)
        if (!form.id || !form.name) {
          setGlobalError('Identifier and name are required')
        }
        return
      }
    } else if (!form.id || !form.name || (currentAts?.base_url !== '' && !form.website)) {
      const isFixedUrl = currentAts?.base_url === ''
      setGlobalError(
        isFixedUrl ? 'Identifier and name are required' : 'Identifier, name, and URL are required'
      )
      return
    }
    setLoading(true)
    try {
      const scanSettings: CreateScanSettings = {
        adapter_type: form.adapter_type,
        active: form.active,
      }
      if (isWorkday) {
        scanSettings.adapter_config = { host: workday.host, site: workday.site }
      }
      if (isEdit && initialData) {
        await updateAdminCompany(initialData.id, { name: form.name, website: form.website })
        // Blank override clears any existing one (null tells the backend to fall
        // back to the global SCAN_INTERVAL_MINUTES default).
        const trimmedInterval = intervalOverride.trim()
        const scanIntervalOverride = trimmedInterval === '' ? null : Number(trimmedInterval)
        await updateAdminCompanyScanSettings(initialData.id, {
          adapter_type: scanSettings.adapter_type,
          active: scanSettings.active,
          scan_interval_override: scanIntervalOverride,
          ...(scanSettings.adapter_config ? { adapter_config: scanSettings.adapter_config } : {}),
        })
        onUpdated?.({ ...initialData, ...form })
      } else {
        const adminCompany = await createAdminCompany({
          id: form.id,
          name: form.name,
          website: form.website,
          scan_settings: scanSettings,
        })
        onCreated?.({
          ...adminCompany,
          url: adminCompany.website ?? '',
          adapter_type: form.adapter_type,
          active: form.active,
          last_scan_at: null,
          last_scan_status: null,
          // adminCompany.scan_settings is the raw ScanSettingsView | null; the
          // Company overlay carries the narrower CompanyScanSettings. A freshly
          // created company has no error state, so leave it undefined.
          scan_settings: undefined,
        })
      }
      handleClose()
    } catch (err: unknown) {
      if (err instanceof ApiValidationError) {
        const fields = err.fieldErrors()
        setFieldErrors(fields)
        // Surface backend errors that don't map to a visible form field
        // (e.g. `slug`) as a global message so the modal isn't silently stuck.
        const visibleFields = new Set([
          'id',
          'name',
          'website',
          'adapter_type',
          'workday_host',
          'workday_site',
        ])
        const unmapped = Object.entries(fields).find(([f]) => !visibleFields.has(f))
        if (unmapped) {
          const [field, message] = unmapped
          setGlobalError(`${field}: ${message}`)
        } else if (Object.keys(fields).length === 0) {
          setGlobalError(err.message || `Failed to ${isEdit ? 'update' : 'create'} company`)
        }
      } else {
        setGlobalError(
          err instanceof Error ? err.message : `Failed to ${isEdit ? 'update' : 'create'} company`
        )
      }
    } finally {
      setLoading(false)
    }
  }

  const urlHint = ADAPTER_HINTS[form.adapter_type]
  // hasTemplate = true when the ATS has a known base_url prefix for auto-fill
  const hasTemplate = !!(currentAts?.base_url && currentAts.base_url !== '')

  return (
    <Dialog.Root open={open} onOpenChange={(v) => !v && handleClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-[#13131a] border border-white/10 rounded-xl shadow-2xl p-6">
          <div className="flex items-center justify-between mb-6">
            <Dialog.Title className="text-lg font-semibold text-white">
              {isEdit ? 'Edit Company' : 'Add Company'}
            </Dialog.Title>
            <button
              onClick={handleClose}
              className="text-slate-400 hover:text-white transition-colors"
            >
              <X size={18} />
            </button>
          </div>
          <Dialog.Description className="sr-only">
            {isEdit
              ? 'Edit an existing company in the JobRadar admin catalog.'
              : 'Add a new company to the JobRadar admin catalog, choosing an adapter and configuring scan settings.'}
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Adapter first — drives URL template */}
            <Field label="Adapter">
              <select
                value={form.adapter_type}
                onChange={(e) => handleAdapterChange(e.target.value)}
                className={inputClass}
              >
                {atsList
                  .filter((a) => a.is_admin_creatable)
                  .map((a) => (
                    <option key={a.name} value={a.name}>
                      {a.display_name}
                    </option>
                  ))}
              </select>
              {fieldErrors.adapter_type && (
                <p className="text-xs text-red-400 mt-1">{fieldErrors.adapter_type}</p>
              )}
            </Field>

            <Field
              label="Identifier (slug)"
              hint={isEdit ? 'Cannot be changed' : 'Unique slug, e.g. stripe'}
            >
              <input
                value={form.id}
                onChange={(e) =>
                  !isEdit && handleIdChange(e.target.value.toLowerCase().replace(/\s+/g, '-'))
                }
                placeholder="stripe"
                disabled={isEdit}
                className={cn(inputClass, isEdit && 'opacity-50 cursor-not-allowed')}
              />
              {fieldErrors.id && <p className="text-xs text-red-400 mt-1">{fieldErrors.id}</p>}
            </Field>

            <Field label="Name">
              <input
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder="Stripe"
                className={inputClass}
              />
              {fieldErrors.name && <p className="text-xs text-red-400 mt-1">{fieldErrors.name}</p>}
            </Field>

            {isWorkday && (
              <>
                <Field label="Workday host" hint="e.g. nvidia.wd5.myworkdayjobs.com">
                  <input
                    value={workday.host}
                    onChange={(e) => handleWorkdayHostChange(e.target.value)}
                    placeholder="nvidia.wd5.myworkdayjobs.com"
                    aria-label="Workday host"
                    className={inputClass}
                  />
                  {fieldErrors.workday_host && (
                    <p className="text-xs text-red-400 mt-1">{fieldErrors.workday_host}</p>
                  )}
                </Field>

                <Field label="Site path" hint="e.g. NVIDIAExternalCareerSite">
                  <input
                    value={workday.site}
                    onChange={(e) => handleWorkdaySiteChange(e.target.value)}
                    placeholder="NVIDIAExternalCareerSite"
                    aria-label="Workday site path"
                    className={inputClass}
                  />
                  {fieldErrors.workday_site && (
                    <p className="text-xs text-red-400 mt-1">{fieldErrors.workday_site}</p>
                  )}
                </Field>
              </>
            )}

            <Field label="Careers URL" hint={isEdit ? undefined : urlHint}>
              <div className="relative">
                <input
                  value={form.website}
                  onChange={(e) => handleUrlChange(e.target.value)}
                  readOnly={isWorkday}
                  placeholder={
                    isWorkday
                      ? 'Auto-filled — enter host + site above'
                      : hasTemplate && !isEdit
                        ? 'Auto-filled — enter Company ID above'
                        : 'https://...'
                  }
                  className={cn(
                    inputClass,
                    'pr-9',
                    hasTemplate && !urlManuallyEdited && form.website && 'text-slate-400',
                    isWorkday && 'text-slate-400 cursor-not-allowed'
                  )}
                />
                <div className="absolute right-2.5 top-1/2 -translate-y-1/2 flex items-center gap-1.5">
                  {hasTemplate && urlManuallyEdited && !isEdit && (
                    <button
                      type="button"
                      onClick={() => {
                        setUrlManuallyEdited(false)
                        setForm((f) => ({
                          ...f,
                          website: buildUrl(
                            atsList.find((a) => a.name === f.adapter_type),
                            f.id
                          ),
                        }))
                      }}
                      className="text-xs text-violet-400 hover:text-violet-300 transition-colors"
                    >
                      Reset
                    </button>
                  )}
                  {form.website && (
                    <a
                      href={form.website}
                      target="_blank"
                      rel="noreferrer"
                      tabIndex={-1}
                      title="Open URL"
                      className="text-slate-500 hover:text-violet-400 transition-colors"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <ExternalLink size={13} />
                    </a>
                  )}
                </div>
              </div>
              {hasTemplate && !urlManuallyEdited && form.website && !isEdit && (
                <p className="text-[11px] text-violet-400/70 mt-1">✓ Auto-filled from identifier</p>
              )}
              {isWorkday && form.website && (
                <p className="text-[11px] text-violet-400/70 mt-1">
                  ✓ Auto-filled from host + site
                </p>
              )}
              {fieldErrors.website && (
                <p className="text-xs text-red-400 mt-1">{fieldErrors.website}</p>
              )}
            </Field>

            <div className="flex items-center justify-between">
              <span className="text-sm text-slate-300">Start scanning immediately</span>
              <button
                type="button"
                role="switch"
                aria-checked={form.active}
                onClick={() => setForm((f) => ({ ...f, active: !f.active }))}
                className={cn(
                  'relative inline-flex w-10 h-6 rounded-full transition-colors duration-200 focus:outline-none shrink-0',
                  form.active ? 'bg-violet-600' : 'bg-white/15'
                )}
              >
                <span
                  className={cn(
                    'absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform duration-200',
                    form.active ? 'translate-x-5' : 'translate-x-1'
                  )}
                />
              </button>
            </div>

            {isEdit && (
              <Field
                label="Scan interval override (minutes)"
                hint="Optional — blank uses the global default"
              >
                <input
                  type="number"
                  min={1}
                  step={1}
                  inputMode="numeric"
                  value={intervalOverride}
                  aria-label="Scan interval override (minutes)"
                  onChange={(e) => {
                    setIntervalOverride(e.target.value)
                    setFieldErrors((errs) => {
                      const updated = { ...errs }
                      delete updated.interval_override
                      return updated
                    })
                  }}
                  placeholder="e.g. 720"
                  className={inputClass}
                />
                {fieldErrors.interval_override && (
                  <p className="text-xs text-red-400 mt-1">{fieldErrors.interval_override}</p>
                )}
              </Field>
            )}

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
                className={cn(
                  btnBase,
                  'flex-1 bg-violet-600 hover:bg-violet-500 text-white disabled:opacity-50'
                )}
              >
                {loading
                  ? isEdit
                    ? 'Saving…'
                    : 'Adding…'
                  : isEdit
                    ? 'Save Changes'
                    : 'Add Company'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
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

const inputClass =
  'w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-violet-500 transition-colors'
const btnBase =
  'px-4 py-2 rounded-lg text-sm font-medium transition-colors cursor-pointer disabled:cursor-not-allowed'
