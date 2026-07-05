package common

import "time"

const (
	// LabelMaxLen bounds the user-facing display label (upload and rename).
	LabelMaxLen = 200
	// OriginalFilenameMaxLen bounds the stored original filename.
	OriginalFilenameMaxLen = 255
	// ResumeLimitPerUser is the max number of resumes a single user may store.
	ResumeLimitPerUser = 50
	// MaxFileSizeBytes is the max accepted upload size (2MB), matching the
	// `resumes.size_bytes` CHECK constraint in migration 086.
	MaxFileSizeBytes = 2 * 1024 * 1024
)

// KeywordExtractionTimeout bounds a single keyword-extraction attempt
// (storage read + text extraction + LLM call). Applied to both the
// best-effort post-upload background extraction (so a hung LLM call cannot
// leak the goroutine indefinitely) and the explicit synchronous retry
// endpoint (so a stuck extraction cannot hang the HTTP request forever).
const KeywordExtractionTimeout = 60 * time.Second
