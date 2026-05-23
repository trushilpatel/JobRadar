import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Pagination } from '@/components/ui/pagination'

describe('Pagination — cursor mode (V4 layout)', () => {
  const baseProps = {
    mode: 'cursor' as const,
    page: 1,
    limit: 20,
    itemsOnPage: 19,
    itemsBeforePage: 0,
    hasNext: true,
    hasPrev: false,
    knownTotal: 6807,
    onNext: vi.fn(),
    onPrev: vi.fn(),
  }

  it('renders the range "X–Y of N" in the middle, not "Page N"', () => {
    render(<Pagination {...baseProps} />)
    expect(screen.getByText('1–19 of 6,807')).toBeInTheDocument()
    expect(screen.queryByText(/^Page /)).not.toBeInTheDocument()
  })

  it('omits the "of N" suffix when knownTotal is undefined', () => {
    render(<Pagination {...baseProps} knownTotal={undefined} />)
    expect(screen.getByText('1–19')).toBeInTheDocument()
  })

  it('shows "No results" when there are no items and no prior pages', () => {
    render(<Pagination {...baseProps} itemsOnPage={0} hasPrev={false} hasNext={false} />)
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  it('shows "Loading…" when loading=true and no items have rendered yet', () => {
    render(<Pagination {...baseProps} itemsOnPage={0} loading />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('does NOT render the « (first) button when onFirst is not provided', () => {
    render(<Pagination {...baseProps} />)
    expect(screen.queryByRole('button', { name: /first page/i })).not.toBeInTheDocument()
  })

  it('renders « when onFirst IS provided, and calls it on click', () => {
    const onFirst = vi.fn()
    render(<Pagination {...baseProps} hasPrev={true} onFirst={onFirst} />)
    const btn = screen.getByRole('button', { name: /first page/i })
    expect(btn).toBeInTheDocument()
    fireEvent.click(btn)
    expect(onFirst).toHaveBeenCalledTimes(1)
  })

  it('« and ‹ are disabled on page 1 (hasPrev=false)', () => {
    const onFirst = vi.fn()
    render(<Pagination {...baseProps} hasPrev={false} onFirst={onFirst} />)
    expect(screen.getByRole('button', { name: /first page/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /previous page/i })).toBeDisabled()
  })

  it('› is disabled when hasNext is false', () => {
    render(<Pagination {...baseProps} hasNext={false} />)
    expect(screen.getByRole('button', { name: /next page/i })).toBeDisabled()
  })

  it('› fires onNext when clicked', () => {
    const onNext = vi.fn()
    render(<Pagination {...baseProps} onNext={onNext} />)
    fireEvent.click(screen.getByRole('button', { name: /next page/i }))
    expect(onNext).toHaveBeenCalledTimes(1)
  })

  it('‹ fires onPrev when clicked', () => {
    const onPrev = vi.fn()
    render(<Pagination {...baseProps} hasPrev={true} onPrev={onPrev} />)
    fireEvent.click(screen.getByRole('button', { name: /previous page/i }))
    expect(onPrev).toHaveBeenCalledTimes(1)
  })

  it('does NOT render a "last page" button (intentional — no » in cursor mode)', () => {
    render(<Pagination {...baseProps} />)
    expect(screen.queryByRole('button', { name: /last page/i })).not.toBeInTheDocument()
  })

  it('Rows selector reflects the current limit and fires onLimitChange', () => {
    const onLimitChange = vi.fn()
    render(<Pagination {...baseProps} onLimitChange={onLimitChange} />)
    const select = screen.getByRole('combobox') as HTMLSelectElement
    expect(select.value).toBe('20')
    fireEvent.change(select, { target: { value: '50' } })
    expect(onLimitChange).toHaveBeenCalledWith(50)
  })

  it('does not show Rows selector when onLimitChange is omitted', () => {
    render(<Pagination {...baseProps} />)
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })
})

describe('Pagination — offset mode', () => {
  const baseProps = {
    mode: 'offset' as const,
    page: 1,
    limit: 20,
    total: 483,
    onPageChange: vi.fn(),
  }

  it('renders range on the left and numbered pages in the center', () => {
    render(<Pagination {...baseProps} />)
    expect(screen.getByText('1–20 of 483')).toBeInTheDocument()
    // Page numbers appear as numeric buttons
    expect(screen.getByRole('button', { name: '1' })).toBeInTheDocument()
  })

  it('shows "No results" when total=0', () => {
    render(<Pagination {...baseProps} total={0} />)
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  it('renders « when onFirst is provided', () => {
    const onFirst = vi.fn()
    render(<Pagination {...baseProps} page={3} onFirst={onFirst} />)
    const btn = screen.getByRole('button', { name: /first page/i })
    fireEvent.click(btn)
    expect(onFirst).toHaveBeenCalled()
  })

  it('does not render » last-page button', () => {
    render(<Pagination {...baseProps} />)
    expect(screen.queryByRole('button', { name: /last page/i })).not.toBeInTheDocument()
  })

  it('clicking a numbered page fires onPageChange with that page', () => {
    const onPageChange = vi.fn()
    render(<Pagination {...baseProps} onPageChange={onPageChange} />)
    // Multiple "1" / "2" buttons may exist; we trust the order from getPages().
    const pageTwo = screen.getByRole('button', { name: '2' })
    fireEvent.click(pageTwo)
    expect(onPageChange).toHaveBeenCalledWith(2)
  })
})
