package dto

// AttachmentView is the response shape for PUT /api/apply-queue/{jobId}/resume
// when a resume is attached (resume_id non-null in the request). A clear
// request (resume_id: null) returns no body — 204 No Content — see
// services.RestService.setResume.
type AttachmentView struct {
	JobID       string  `json:"job_id"`
	ResumeID    *string `json:"resume_id"`
	ResumeLabel *string `json:"resume_label"`
}

// SetResumeInput is the request body for PUT /api/apply-queue/{jobId}/resume.
// ResumeID nil clears any existing attachment for the job.
type SetResumeInput struct {
	ResumeID *string `json:"resume_id"`
}
