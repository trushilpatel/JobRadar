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
	// PresignTTL is how long a download URL stays valid after issuance.
	PresignTTL = 5 * time.Minute
)
