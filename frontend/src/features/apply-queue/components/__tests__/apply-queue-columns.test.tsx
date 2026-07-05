import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { buildApplyQueueColumns } from '../apply-queue-columns'
import type { ApplyQueueItem } from '../../api/apply-queue.api'

describe('buildApplyQueueColumns', () => {
  const actions = {
    resumeOptions: [{ id: 'resume-1', label: 'Resume One' }],
    onResumeChange: vi.fn(),
    isResumeUpdatePending: vi.fn().mockReturnValue(false),
  }

  it('builds exactly the job, status, and resume columns', () => {
    const columns = buildApplyQueueColumns(actions)
    expect(columns.map((c) => c.id)).toEqual(['job', 'status', 'resume'])
  })

  it('disables sorting on the status and resume columns', () => {
    const columns = buildApplyQueueColumns(actions)
    const status = columns.find((c) => c.id === 'status')
    const resume = columns.find((c) => c.id === 'resume')
    expect(status?.enableSorting).toBe(false)
    expect(resume?.enableSorting).toBe(false)
  })

  describe('resume column recommendation wiring', () => {
    const item: ApplyQueueItem = {
      job_id: 'job-1',
      job_title: 'Senior Engineer',
      company: 'Acme',
      job_url: 'https://acme.example/jobs/1',
      status: 'saved',
      resume_id: null,
      resume_label: null,
      recommended_resume_id: 'resume-1',
      recommended_resume_label: 'Resume One',
    }

    function TestTable({ data }: { data: ApplyQueueItem[] }) {
      const columns = buildApplyQueueColumns(actions)
      const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() })
      const resumeCell = table.getRowModel().rows[0]!.getVisibleCells()[2]!
      return <>{flexRender(resumeCell.column.columnDef.cell, resumeCell.getContext())}</>
    }

    it('marks the option matching recommended_resume_id as recommended', async () => {
      const user = userEvent.setup()
      render(<TestTable data={[item]} />)
      await user.click(screen.getByRole('combobox'))
      expect(await screen.findByText('Recommended')).toBeInTheDocument()
    })

    it('does not mark any option recommended when recommended_resume_id is null', async () => {
      const user = userEvent.setup()
      render(<TestTable data={[{ ...item, recommended_resume_id: null }]} />)
      await user.click(screen.getByRole('combobox'))
      expect(await screen.findByText('Resume One')).toBeInTheDocument()
      expect(screen.queryByText('Recommended')).not.toBeInTheDocument()
    })
  })
})
