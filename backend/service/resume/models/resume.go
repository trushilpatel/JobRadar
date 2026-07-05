// Package resumemodel holds the GORM model for the resumes table.
package resumemodel

import "time"

// Resume mirrors the `resumes` table (migration 086). Each row is a single
// uploaded file — no versioning in this phase. StorageKey is deliberately
// excluded from JSON (json:"-"): it's an internal object-storage path, never
// something a client should see or depend on.
type Resume struct {
	ID               string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID           string    `gorm:"column:user_id;not null;index"                  json:"user_id"`
	Label            string    `gorm:"not null"                                       json:"label"`
	OriginalFilename string    `gorm:"column:original_filename;not null"              json:"original_filename"`
	StorageKey       string    `gorm:"column:storage_key;not null;uniqueIndex"        json:"-"`
	ContentType      string    `gorm:"column:content_type;not null"                   json:"content_type"`
	SizeBytes        int64     `gorm:"column:size_bytes;not null"                     json:"size_bytes"`
	CreatedAt        time.Time `gorm:"column:created_at;not null;default:now()"       json:"created_at"`
}

func (Resume) TableName() string { return "resumes" }
