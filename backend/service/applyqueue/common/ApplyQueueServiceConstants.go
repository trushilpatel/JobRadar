// Package common holds constants owned by the apply-queue domain.
package common

// Filter values accepted by GET /api/apply-queue?filter=.
//
//   - FilterAll (default): saved_at IS NOT NULL OR status IN (applied,
//     interviewing) — the set of jobs the user is actively engaging with.
//   - FilterSaved: saved_at IS NOT NULL only — the orthogonal bookmark flag,
//     matching jobcommon.JobStatusSaved's "saved is orthogonal to workflow
//     status" semantics used across the job domain.
const (
	FilterAll   = "all"
	FilterSaved = "saved"
)
