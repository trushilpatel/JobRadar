import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { Toaster } from 'sonner'
import { showDismissStripe } from '@/components/ui/dismiss-stripe'
import { hiddenItems } from '@/lib/hooks/hidden-items'

function renderToaster() {
  return render(<Toaster />)
}

describe('showDismissStripe', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the headline and submessage', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({ message: 'Dismissed', submessage: 'Senior Engineer at Stripe' })
    })
    expect(await screen.findByText('Dismissed')).toBeInTheDocument()
    expect(screen.getByText('Senior Engineer at Stripe')).toBeInTheDocument()
  })

  it('shows Undo button when onUndo is provided', async () => {
    renderToaster()
    const onUndo = vi.fn()
    act(() => {
      showDismissStripe({ message: 'Dismissed', onUndo })
    })
    const btn = await screen.findByRole('button', { name: /undo/i })
    expect(btn).toBeInTheDocument()
    fireEvent.click(btn)
    expect(onUndo).toHaveBeenCalledTimes(1)
  })

  it('hides Undo button when onUndo is not provided', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({ message: 'Dismissed' })
    })
    await screen.findByText('Dismissed')
    expect(screen.queryByRole('button', { name: /undo/i })).not.toBeInTheDocument()
  })

  it('shows the "Don\'t show ... again" checkbox when hideTarget is provided', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({
        message: 'Dismissed',
        hideTarget: { category: 'companies', value: 'Stripe' },
      })
    })
    expect(await screen.findByRole('checkbox')).toBeInTheDocument()
    // Both the static text node and the inner label span carry "Stripe".
    const labelMatches = screen.getAllByText(/Stripe/)
    expect(labelMatches.length).toBeGreaterThan(0)
  })

  it('omits the checkbox when no hideTarget is provided', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({ message: 'Dismissed' })
    })
    await screen.findByText('Dismissed')
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('uses hideTarget.label in the copy when provided', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({
        message: 'Dismissed',
        hideTarget: { category: 'companies', value: 'stripe-inc', label: 'Stripe' },
      })
    })
    await screen.findByRole('checkbox')
    expect(screen.getAllByText('Stripe').length).toBeGreaterThan(0)
  })

  it('adds to hidden-items when the checkbox is checked', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({
        message: 'Dismissed',
        hideTarget: { category: 'companies', value: 'Stripe' },
      })
    })
    const cb = await screen.findByRole('checkbox')
    expect(hiddenItems.has('companies', 'Stripe')).toBe(false)
    fireEvent.click(cb)
    expect(hiddenItems.has('companies', 'Stripe')).toBe(true)
  })

  it('removes from hidden-items when the checkbox is unchecked', async () => {
    renderToaster()
    act(() => {
      showDismissStripe({
        message: 'Dismissed',
        hideTarget: { category: 'companies', value: 'Stripe' },
      })
    })
    const cb = (await screen.findByRole('checkbox')) as HTMLInputElement
    fireEvent.click(cb)
    expect(hiddenItems.has('companies', 'Stripe')).toBe(true)
    fireEvent.click(cb)
    expect(hiddenItems.has('companies', 'Stripe')).toBe(false)
  })

  it('Undo reverts the hide checkbox effect too', async () => {
    renderToaster()
    const onUndo = vi.fn()
    act(() => {
      showDismissStripe({
        message: 'Dismissed',
        onUndo,
        hideTarget: { category: 'companies', value: 'Stripe' },
      })
    })
    const cb = await screen.findByRole('checkbox')
    fireEvent.click(cb)
    expect(hiddenItems.has('companies', 'Stripe')).toBe(true)

    const undo = screen.getByRole('button', { name: /undo/i })
    fireEvent.click(undo)
    expect(onUndo).toHaveBeenCalledTimes(1)
    expect(hiddenItems.has('companies', 'Stripe')).toBe(false)
  })
})
