import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Button, buttonVariants } from '@/components/ui/button'

describe('Button', () => {
  it('renders a native button with default variant/size classes', () => {
    render(<Button>Click</Button>)
    const btn = screen.getByRole('button', { name: 'Click' })
    expect(btn.tagName).toBe('BUTTON')
    expect(btn.className).toContain('bg-panel')
    expect(btn.className).toContain('h-10')
  })

  it('fires onClick', async () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Go</Button>)
    await userEvent.click(screen.getByRole('button', { name: 'Go' }))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('renders the destructive variant and icon size', () => {
    render(
      <Button variant="destructive" size="icon" aria-label="del">
        x
      </Button>
    )
    const btn = screen.getByRole('button', { name: 'del' })
    expect(btn.className).toContain('bg-red-500')
    expect(btn.className).toContain('w-10')
  })

  it('is disabled when the disabled prop is set', () => {
    render(<Button disabled>Nope</Button>)
    expect(screen.getByRole('button', { name: 'Nope' })).toBeDisabled()
  })

  it('renders as a child element via asChild (Slot)', () => {
    render(
      <Button asChild>
        <a href="/x">Link</a>
      </Button>
    )
    const link = screen.getByRole('link', { name: 'Link' })
    expect(link.tagName).toBe('A')
    expect(link.className).toContain('bg-panel')
  })

  it('merges a custom className', () => {
    render(<Button className="extra-x">Hi</Button>)
    expect(screen.getByRole('button', { name: 'Hi' }).className).toContain('extra-x')
  })

  it('exposes a buttonVariants helper', () => {
    expect(buttonVariants({ variant: 'outline', size: 'sm' })).toContain('border-[var(--border)]')
  })
})
