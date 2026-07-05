import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'
import type { Resume } from '../types'

const ResumeSchema = z.object({
  id: z.string(),
  user_id: z.string(),
  label: z.string(),
  original_filename: z.string(),
  content_type: z.union([
    z.literal('application/pdf'),
    z.literal('application/vnd.openxmlformats-officedocument.wordprocessingml.document'),
  ]),
  size_bytes: z.number(),
  created_at: z.string(),
})
const ResumeListSchema = z.array(ResumeSchema)

export async function fetchResumes(): Promise<Resume[]> {
  const raw = await apiClient.get<unknown>('/resumes')
  return ResumeListSchema.parse(raw) as Resume[]
}

export async function uploadResume(file: File, label: string): Promise<Resume> {
  const form = new FormData()
  form.append('file', file, file.name)
  if (label) form.append('label', label)
  const raw = await apiClient.upload<unknown>('/resumes', form)
  return ResumeSchema.parse(raw) as Resume
}

export async function getResumeDownloadUrl(id: string): Promise<string> {
  const raw = await apiClient.get<{ url: string }>(`/resumes/${encodeURIComponent(id)}/download`)
  return raw.url
}

export async function renameResume(id: string, label: string): Promise<Resume> {
  const raw = await apiClient.patch<unknown>(`/resumes/${encodeURIComponent(id)}`, { label })
  return ResumeSchema.parse(raw) as Resume
}

export async function deleteResume(id: string): Promise<void> {
  await apiClient.delete<void>(`/resumes/${encodeURIComponent(id)}`)
}
