package dto

// ResumeKeywordInput is the plain-text resume body sent to the LLM for
// technology/skill keyword extraction (see service/resume/textextract for
// how Text is produced from the uploaded PDF/DOCX bytes).
type ResumeKeywordInput struct {
	Text string
}
