package rolemodel

import "time"

// Role mirrors the `roles` table created in migration 043.
type Role struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	Name        string    `gorm:"not null"             json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Role) TableName() string { return "roles" }

// UserRole mirrors the `user_roles` join table created in migration 043.
type UserRole struct {
	UserID    string    `gorm:"primaryKey;type:uuid" json:"user_id"`
	RoleID    string    `gorm:"primaryKey;type:text" json:"role_id"`
	GrantedAt time.Time `json:"granted_at"`
	GrantedBy *string   `gorm:"type:uuid"            json:"granted_by"`
}

func (UserRole) TableName() string { return "user_roles" }

// Seeded role IDs — match the rows inserted by migration 043.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)
