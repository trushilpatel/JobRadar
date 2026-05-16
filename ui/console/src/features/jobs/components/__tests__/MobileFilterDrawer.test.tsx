import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// vaul's pointer/drag infrastructure crashes in jsdom because some computed
// transform styles come back undefined. Replace it with a minimal render-only
// stand-in so we can assert on content + callbacks without exercising drag.
vi.mock('vaul', () => {
  type WithChildren = { children?: React.ReactNode }
  const Root = ({
    open,
    children,
  }: { open?: boolean; onOpenChange?: (o: boolean) => void } & WithChildren) =>
    open ? <div data-testid="drawer-root">{children}</div> : null
  const Portal = ({ children }: WithChildren) => <>{children}</>
  const Overlay = (props: WithChildren) => <div {...props} />
  const Content = ({ children, ...rest }: WithChildren) => (
    <div role="dialog" {...rest}>
      {children}
    </div>
  )
  const Trigger = ({ children, ...rest }: WithChildren) => <button {...rest}>{children}</button>
  const Title = ({ children, ...rest }: WithChildren) => <h2 {...rest}>{children}</h2>
  const Description = ({ children, ...rest }: WithChildren) => <p {...rest}>{children}</p>
  const Close = ({
    children,
    asChild,
    ...rest
  }: WithChildren & { asChild?: boolean; onClick?: () => void }) => {
    void asChild
    return <span {...rest}>{children}</span>
  }
  return { Drawer: { Root, Portal, Overlay, Content, Trigger, Title, Description, Close } }
})

import { MobileFilterDrawer } from '../MobileFilterDrawer'

beforeEach(() => {
  cleanup()
})

describe('MobileFilterDrawer', () => {
  it('renders nothing when closed', () => {
    render(<MobileFilterDrawer open={false} onOpenChange={() => {}} />)
    expect(screen.queryByText('Filters')).not.toBeInTheDocument()
  })

  it('renders title when open', () => {
    render(<MobileFilterDrawer open={true} onOpenChange={() => {}} />)
    expect(screen.getByText('Filters')).toBeInTheDocument()
  })

  it('renders all filter sections when open', () => {
    render(
      <MobileFilterDrawer
        open={true}
        onOpenChange={() => {}}
        onCompaniesChange={() => {}}
        onExpChange={() => {}}
        onDateChange={() => {}}
        onLocationChange={() => {}}
      />
    )
    expect(screen.getByText('Companies')).toBeInTheDocument()
    expect(screen.getByText('Experience')).toBeInTheDocument()
    expect(screen.getByText('Posted')).toBeInTheDocument()
    expect(screen.getByText('Location')).toBeInTheDocument()
  })

  it('toggles an exp pill on click', async () => {
    const onExpChange = vi.fn()
    render(
      <MobileFilterDrawer
        open={true}
        onOpenChange={() => {}}
        expFilter={[]}
        onExpChange={onExpChange}
      />
    )
    await userEvent.click(screen.getByText('2-5 yrs'))
    expect(onExpChange).toHaveBeenCalledWith(['2-5'])
  })

  it('toggles a date pill on click', async () => {
    const onDateChange = vi.fn()
    render(
      <MobileFilterDrawer
        open={true}
        onOpenChange={() => {}}
        dateFilter="all"
        onDateChange={onDateChange}
      />
    )
    await userEvent.click(screen.getByText('7d'))
    expect(onDateChange).toHaveBeenCalledWith('7d')
  })

  it('Reset button calls onReset', async () => {
    const onReset = vi.fn()
    render(<MobileFilterDrawer open={true} onOpenChange={() => {}} onReset={onReset} />)
    await userEvent.click(screen.getByText('Reset'))
    expect(onReset).toHaveBeenCalled()
  })
})
