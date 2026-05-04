package dto

import (
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

// JobView is the per-user view of a single Job — the global Job record
// plus the caller's UserJobState overlay (may be empty when the user has
// never interacted with the job, in which case Status defaults to "new").
type JobView struct {
	jobmodel.Job
	UserState UserJobStateView `json:"user_state"`
}

// UserJobStateView is the read-shape for per-user job interaction state.
// It mirrors UserJobState but omits internal timestamps not relevant to UI
// callers; an empty value (Status == "new") represents "never interacted".
type UserJobStateView struct {
	Status          string  `json:"status"`
	Notes           *string `json:"notes,omitempty"`
	DismissalReason *string `json:"dismissal_reason,omitempty"`
	DismissalNote   *string `json:"dismissal_note,omitempty"`
	SavedAt         *string `json:"saved_at,omitempty"`
	AppliedAt       *string `json:"applied_at,omitempty"`
	DismissedAt     *string `json:"dismissed_at,omitempty"`
}
