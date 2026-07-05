// Package applyqueuemodel holds the GORM model for the
// resume_job_attachments table.
package applyqueuemodel

import "time"

// ResumeJobAttachment mirrors the resume_job_attachments table (migration
// 088). One row per (user_id, job_id) — at most one resume may be attached
// to a given job for a given user at a time; attaching a different resume
// overwrites the row via upsert (ON CONFLICT (user_id, job_id) DO UPDATE)
// rather than creating a second row.
type ResumeJobAttachment struct {
	UserID   string `gorm:"primaryKey;column:user_id" json:"user_id"`
	JobID    string `gorm:"primaryKey;column:job_id"  json:"job_id"`
	ResumeID string `gorm:"column:resume_id;not null;type:uuid" json:"resume_id"`

	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

// TableName pins ResumeJobAttachment to the resume_job_attachments table.
func (ResumeJobAttachment) TableName() string {
	return "resume_job_attachments"
}
