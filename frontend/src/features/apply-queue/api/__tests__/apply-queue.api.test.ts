import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import { fetchApplyQueue, setApplyQueueResume } from '../apply-queue.api'
import { setSchemaDriftLogger } from '@/lib/core/resilient-parse'

vi.mock('@/lib/core/api-client')

const validItem = {
  job_id: 'job-1',
  job_title: 'Senior Engineer',
  company: 'Acme',
  job_url: 'https://acme.example/jobs/1',
  status: 'saved',
  resume_id: null,
  resume_label: null,
}

describe('apply-queue API', () => {
  beforeEach(() => vi.clearAllMocks())

  describe('fetchApplyQueue', () => {
    it('calls the correct endpoint with filter and limit, no cursor param when empty', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ items: [], next_cursor: '' })
      await fetchApplyQueue('all', '', 20)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('/apply-queue?')
      expect(url).toContain('filter=all')
      expect(url).toContain('limit=20')
      expect(url).not.toContain('cursor=')
    })

    it('appends cursor when non-empty', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ items: [], next_cursor: '' })
      await fetchApplyQueue('saved', 'cursor-abc', 50)
      const [url] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(url).toContain('filter=saved')
      expect(url).toContain('cursor=cursor-abc')
      expect(url).toContain('limit=50')
    })

    it('passes the abort signal through when provided', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ items: [], next_cursor: '' })
      const controller = new AbortController()
      await fetchApplyQueue('all', '', 20, controller.signal)
      const [, init] = vi.mocked(apiClient.get).mock.calls[0]!
      expect(init).toEqual({ signal: controller.signal })
    })

    it('parses a valid page of items', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ items: [validItem], next_cursor: 'next-1' })
      const page = await fetchApplyQueue('all')
      expect(page.items).toHaveLength(1)
      expect(page.items[0]).toEqual(validItem)
      expect(page.next_cursor).toBe('next-1')
    })

    it('drops an individual malformed item instead of throwing (resilient parse)', async () => {
      const prevLogger = setSchemaDriftLogger(() => {})
      try {
        vi.mocked(apiClient.get).mockResolvedValue({
          items: [validItem, { job_id: 'bad', status: 'not-a-real-status' }],
          next_cursor: '',
        })
        const page = await fetchApplyQueue('all')
        expect(page.items).toHaveLength(1)
        expect(page.items[0]?.job_id).toBe('job-1')
      } finally {
        setSchemaDriftLogger(prevLogger)
      }
    })

    it('falls back to empty items and empty cursor when the envelope is garbage', async () => {
      vi.mocked(apiClient.get).mockResolvedValue({ nonsense: true })
      const page = await fetchApplyQueue('all')
      expect(page.items).toEqual([])
      expect(page.next_cursor).toBe('')
    })

    it('propagates rejection when apiClient.get rejects', async () => {
      vi.mocked(apiClient.get).mockRejectedValue(new Error('Network error'))
      await expect(fetchApplyQueue('all')).rejects.toThrow('Network error')
    })
  })

  describe('setApplyQueueResume', () => {
    it('calls PUT with a resume_id', async () => {
      vi.mocked(apiClient.put).mockResolvedValue(undefined)
      await setApplyQueueResume('job-1', 'resume-1')
      expect(apiClient.put).toHaveBeenCalledWith('/apply-queue/job-1/resume', {
        resume_id: 'resume-1',
      })
    })

    it('calls PUT with resume_id null to clear the selection', async () => {
      vi.mocked(apiClient.put).mockResolvedValue(undefined)
      await setApplyQueueResume('job-1', null)
      expect(apiClient.put).toHaveBeenCalledWith('/apply-queue/job-1/resume', {
        resume_id: null,
      })
    })

    it('URL-encodes the jobId', async () => {
      vi.mocked(apiClient.put).mockResolvedValue(undefined)
      await setApplyQueueResume('job with space', null)
      const [url] = vi.mocked(apiClient.put).mock.calls[0]!
      expect(url).toContain('job%20with%20space')
    })

    it('propagates rejection on failure', async () => {
      vi.mocked(apiClient.put).mockRejectedValue(new Error('Network error'))
      await expect(setApplyQueueResume('job-1', null)).rejects.toThrow('Network error')
    })
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})
