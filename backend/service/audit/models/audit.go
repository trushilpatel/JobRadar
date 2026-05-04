package auditmodel

import (
	"time"

	"gorm.io/datatypes"
)

// Entry mirrors the `admin_audit_log` table created in migration 043.
type Entry struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	ActorUserID *string        `gorm:"type:uuid"                                      json:"actor_user_id"`
	Action      string         `gorm:"not null"                                       json:"action"`
	TargetType  *string        `json:"target_type"`
	TargetID    *string        `json:"target_id"`
	Payload     datatypes.JSON `json:"payload"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (Entry) TableName() string { return "admin_audit_log" }

// Action constants for now (extend in later phases).
const (
	ActionUserDelete       = "user.delete"
	ActionUserRoleGrant    = "user.role.grant"
	ActionUserRoleRevoke   = "user.role.revoke"
	ActionUserStatusChange = "user.status.change"
)
