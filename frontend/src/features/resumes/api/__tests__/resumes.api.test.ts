import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiClient } from '@/lib/core/api-client'
import {
  fetchResumes,
  uploadResume,
  downloadResume,
  renameResume,
  deleteResume,
} from '../resumes.api'

vi.mock('@/lib/core/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
    upload: vi.fn(),
    getBlob: vi.fn(),
  },
}))

const mockGet = vi.mocked(apiClient.get)
const mockUpload = vi.mocked(apiClient.upload)
const mockPatch = vi.mocked(apiClient.patch)
const mockDelete = vi.mocked(apiClient.delete)
const mockGetBlob = vi.mocked(apiClient.getBlob)

const sampleResume = {
  id: 'r1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf',
  size_bytes: 1024,
  created_at: '2026-07-03T00:00:00Z',
}

describe('resumes.api', () => {
  beforeEach(() => vi.clearAllMocks())

  describe('fetchResumes', () => {
    it('parses a valid list', async () => {
      mockGet.mockResolvedValue([sampleResume])
      const result = await fetchResumes()
      expect(result).toHaveLength(1)
      expect(result[0]!.id).toBe('r1')
    })

    it('throws on a malformed item', async () => {
      mockGet.mockResolvedValue([{ id: 'r1' }])
      await expect(fetchResumes()).rejects.toThrow()
    })
  })

  describe('uploadResume', () => {
    it('builds FormData with file and label', async () => {
      mockUpload.mockResolvedValue(sampleResume)
      const file = new File(['data'], 'resume.pdf', { type: 'application/pdf' })
      await uploadResume(file, 'My resume')
      const [path, form] = mockUpload.mock.calls[0]!
      expect(path).toBe('/resumes')
      expect((form as FormData).get('label')).toBe('My resume')
      expect(((form as FormData).get('file') as File).name).toBe('resume.pdf')
    })

    it('omits the label field when blank', async () => {
      mockUpload.mockResolvedValue(sampleResume)
      const file = new File(['data'], 'resume.pdf', { type: 'application/pdf' })
      await uploadResume(file, '')
      const [, form] = mockUpload.mock.calls[0]!
      expect((form as FormData).get('label')).toBeNull()
    })
  })

  describe('downloadResume', () => {
    it('url-encodes the id and returns the blob', async () => {
      const blob = new Blob(['%PDF-1.4'], { type: 'application/pdf' })
      mockGetBlob.mockResolvedValue(blob)
      const result = await downloadResume('r/1')
      expect(mockGetBlob).toHaveBeenCalledWith('/resumes/r%2F1/download')
      expect(result).toBe(blob)
    })
  })

  describe('renameResume', () => {
    it('sends the new label', async () => {
      mockPatch.mockResolvedValue(sampleResume)
      await renameResume('r1', 'New label')
      expect(mockPatch).toHaveBeenCalledWith('/resumes/r1', { label: 'New label' })
    })
  })

  describe('deleteResume', () => {
    it('calls DELETE with the encoded id', async () => {
      mockDelete.mockResolvedValue(undefined)
      await deleteResume('r/1')
      expect(mockDelete).toHaveBeenCalledWith('/resumes/r%2F1')
    })
  })
})
