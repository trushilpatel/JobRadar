import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AdapterConfigCell } from '../components/AdapterConfigCell'

const onSaveMock = vi.fn()

describe('AdapterConfigCell', () => {
  it('displays minutes as hours when value is 60 or more', async () => {
    vi.mocked(onSaveMock).mockResolvedValue(undefined)
    render(<AdapterConfigCell value={1440} min={1} max={10080} unit="min" onSave={onSaveMock} />)
    expect(screen.getByText('24h')).toBeInTheDocument()
  })

  it('displays mixed hours and minutes when not a round hour', async () => {
    render(<AdapterConfigCell value={90} min={1} max={10080} unit="min" onSave={onSaveMock} />)
    expect(screen.getByText('1h 30m')).toBeInTheDocument()
  })

  it('displays minutes directly when under 60', async () => {
    render(<AdapterConfigCell value={30} min={1} max={10080} unit="min" onSave={onSaveMock} />)
    expect(screen.getByText('30min')).toBeInTheDocument()
  })

  it('renders non-minute unit values without formatting', () => {
    render(<AdapterConfigCell value={30} min={0} max={3600} unit="s" onSave={vi.fn()} />)
    expect(screen.getByText('30 s')).toBeInTheDocument()
  })

  it('enters edit mode when clicked', async () => {
    const user = userEvent.setup()
    render(<AdapterConfigCell value={30} min={0} max={3600} unit="s" onSave={vi.fn()} />)
    const btn = screen.getByRole('button')
    await user.click(btn)
    expect(screen.getByRole('spinbutton')).toBeInTheDocument()
  })

  it('calls onSave with parsed value on Enter', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AdapterConfigCell value={30} min={0} max={3600} unit="s" onSave={onSave} />)
    await user.click(screen.getByRole('button'))

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '60')
    await user.keyboard('{Enter}')

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(60))
  })

  it('does not call onSave when value is unchanged', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<AdapterConfigCell value={30} min={0} max={3600} onSave={onSave} />)
    await user.click(screen.getByRole('button'))

    const input = screen.getByRole('spinbutton')
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(onSave).not.toHaveBeenCalled())
  })

  it('shows inline error for out-of-range values', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<AdapterConfigCell value={30} min={1} max={3600} onSave={onSave} />)
    await user.click(screen.getByRole('button'))

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '0')
    await user.keyboard('{Enter}')

    await waitFor(() => expect(screen.getByText(/Must be 1–3600/)).toBeInTheDocument())
    expect(onSave).not.toHaveBeenCalled()
  })

  it('reverts to original value on save error', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('Server error'))
    const user = userEvent.setup()
    render(<AdapterConfigCell value={30} min={0} max={3600} onSave={onSave} />)
    await user.click(screen.getByRole('button'))

    const input = screen.getByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '60')
    await user.keyboard('{Enter}')

    await waitFor(() => expect(screen.getByText(/Save failed/)).toBeInTheDocument())
  })

  it('returns to display mode on Escape', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<AdapterConfigCell value={30} min={0} max={3600} onSave={onSave} />)
    await user.click(screen.getByRole('button'))

    const input = screen.getByRole('spinbutton')
    act(() => {
      input.focus()
    })
    fireEvent.keyDown(input, { key: 'Escape', code: 'Escape' })

    await waitFor(() => expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument())
    expect(screen.getByRole('button')).toBeInTheDocument()
  })
})
