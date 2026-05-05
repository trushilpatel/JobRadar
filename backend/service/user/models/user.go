package usermodel

import "time"

// User mirrors the `users` table created in migration 043.
type User struct {
	ID          string    `gorm:"primaryKey;type:uuid"          json:"id"`
	Email       string    `gorm:"uniqueIndex;type:citext;not null" json:"email"`
	DisplayName *string   `json:"display_name"`
	Status      string    `gorm:"not null;default:active"       json:"status"`
	CreatedAt   time.Time `gorm:"not null;default:now()"        json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()"        json:"updated_at"`
}

func (User) TableName() string { return "users" }

// Status values mirrored from the CHECK constraint on users.status.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)
