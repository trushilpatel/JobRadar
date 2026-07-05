import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ResumePickerCell } from '../ResumePickerCell'

const options = [
  { id: 'resume-1', label: 'Software Engineer Resume' },
  { id: 'resume-2', label: 'Backend Resume' },
]

describe('ResumePickerCell', () => {
  it('shows the placeholder when no resume is selected', () => {
    render(
      <ResumePickerCell
        jobId="job-1"
        resumeId={null}
        resumeLabel={null}
        options={options}
        onChange={vi.fn()}
      />
    )
    expect(screen.getByText('No resume selected')).toBeInTheDocument()
  })

  it('shows the attached resume label when one is selected', () => {
    render(
      <ResumePickerCell
        jobId="job-1"
        resumeId="resume-1"
        resumeLabel="Software Engineer Resume"
        options={options}
        onChange={vi.fn()}
      />
    )
    expect(screen.getByRole('combobox')).toHaveTextContent('Software Engineer Resume')
  })

  it('falls back to resumeLabel when the attached resume is not in the options list', () => {
    // Simulates a resume that was deleted after being attached to this job.
    render(
      <ResumePickerCell
        jobId="job-1"
        resumeId="deleted-resume"
        resumeLabel="Old Resume (deleted)"
        options={options}
        onChange={vi.fn()}
      />
    )
    expect(screen.getByRole('combobox')).toHaveTextContent('Old Resume (deleted)')
  })

  it('calls onChange with the selected resume id when an option is picked', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <ResumePickerCell
        jobId="job-1"
        resumeId={null}
        resumeLabel={null}
        options={options}
        onChange={onChange}
      />
    )

    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByText('Backend Resume'))

    expect(onChange).toHaveBeenCalledWith('job-1', 'resume-2')
  })

  it('calls onChange with null when "No resume selected" is picked to clear', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <ResumePickerCell
        jobId="job-1"
        resumeId="resume-1"
        resumeLabel="Software Engineer Resume"
        options={options}
        onChange={onChange}
      />
    )

    await user.click(screen.getByRole('combobox'))
    await user.click(
      await screen.findAllByText('No resume selected').then((els) => els[els.length - 1]!)
    )

    expect(onChange).toHaveBeenCalledWith('job-1', null)
  })

  it('disables the trigger when disabled is true', () => {
    render(
      <ResumePickerCell
        jobId="job-1"
        resumeId={null}
        resumeLabel={null}
        options={options}
        onChange={vi.fn()}
        disabled
      />
    )
    expect(screen.getByRole('combobox')).toBeDisabled()
  })
})
