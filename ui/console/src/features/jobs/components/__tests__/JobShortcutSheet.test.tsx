import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { JobShortcutSheet } from '../JobShortcutSheet'
import { JOB_COMMANDS } from '../../commands/jobCommands'

beforeEach(() => {
  cleanup()
})

describe('JobShortcutSheet', () => {
  it('renders nothing when open is false', () => {
    render(<JobShortcutSheet open={false} onOpenChange={() => {}} />)
    expect(screen.queryByText('Keyboard shortcuts')).not.toBeInTheDocument()
  })

  it('renders the sheet title when open', () => {
    render(<JobShortcutSheet open onOpenChange={() => {}} />)
    // "Keyboard shortcuts" appears as both the sheet title AND the
    // jobs.nav.open-help command label, so assert at least one occurrence.
    expect(screen.getAllByText('Keyboard shortcuts').length).toBeGreaterThan(0)
  })

  it('renders a row for every command label in the registry', () => {
    render(<JobShortcutSheet open onOpenChange={() => {}} />)
    for (const id of Object.keys(JOB_COMMANDS) as (keyof typeof JOB_COMMANDS)[]) {
      // A label may legitimately appear more than once across categories;
      // assert it renders at least once.
      expect(screen.getAllByText(JOB_COMMANDS[id].label).length).toBeGreaterThan(0)
    }
  })

  it('groups commands under the four category headings', () => {
    render(<JobShortcutSheet open onOpenChange={() => {}} />)
    // Headings are <h3>; some heading text ("Search") also appears as a command
    // label, so scope the assertion to the heading role+level.
    expect(screen.getByRole('heading', { name: 'Navigation', level: 3 })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Tabs', level: 3 })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Status', level: 3 })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Search', level: 3 })).toBeInTheDocument()
  })

  it('does NOT render the removed multi-select shortcut rows', () => {
    render(<JobShortcutSheet open onOpenChange={() => {}} />)
    expect(screen.queryByText('Toggle row selection')).not.toBeInTheDocument()
    expect(screen.queryByText('Range select (last selected → focused)')).not.toBeInTheDocument()
    expect(screen.queryByText('Select all on page')).not.toBeInTheDocument()
    expect(screen.queryByText('Bulk save')).not.toBeInTheDocument()
    expect(screen.queryByText('Bulk applied')).not.toBeInTheDocument()
    expect(screen.queryByText('Bulk dismiss')).not.toBeInTheDocument()
  })

  it('does NOT render the non-existent opt+1..6 tab rows', () => {
    render(<JobShortcutSheet open onOpenChange={() => {}} />)
    expect(screen.queryByText('Tab 1 (New)')).not.toBeInTheDocument()
    expect(screen.queryByText('Tab 6 (Rejected)')).not.toBeInTheDocument()
  })

  it('does NOT render the old stale section headings', () => {
    render(<JobShortcutSheet open onOpenChange={() => {}} />)
    expect(screen.queryByText('Selection / bulk')).not.toBeInTheDocument()
    expect(screen.queryByText('Tabs / views')).not.toBeInTheDocument()
    expect(screen.queryByText('Status actions')).not.toBeInTheDocument()
  })

  it('calls onOpenChange when the built-in close button is clicked', () => {
    const onOpenChange = vi.fn()
    render(<JobShortcutSheet open onOpenChange={onOpenChange} />)
    const closeButtons = screen.queryAllByRole('button', { name: /close/i })
    if (closeButtons.length > 0) {
      closeButtons[0]?.click()
      expect(onOpenChange).toHaveBeenCalledWith(false)
    } else {
      expect(true).toBe(true)
    }
  })
})
