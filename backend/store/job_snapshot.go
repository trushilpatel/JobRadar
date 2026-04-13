package store

import (
	"time"

	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// SnapshotInput carries the data the enricher writes at pipeline time.
type SnapshotInput struct {
	ID                string
	CompanyID         string
	Country           string
	State             string
	Remote            bool
	Sponsorship       bool
	RequiresCitizen   bool
	SecurityClearance bool
	EmployType        string
	ExperienceYears   *int
	TechStack         []byte // raw JSONB bytes
	RoleCategory      string
}

// SnapshotWriter is used by the enricher to record each enriched job.
type SnapshotWriter interface {
	InsertSnapshot(input SnapshotInput) error
	UpdateSnapshotAction(id, action string) error
}

type snapshotStore struct {
	db *gorm.DB
}

func NewSnapshotStore(db *gorm.DB) SnapshotWriter {
	return &snapshotStore{db: db}
}

func (s *snapshotStore) InsertSnapshot(input SnapshotInput) error {
	row := model.JobSnapshot{
		ID:                input.ID,
		CompanyID:         input.CompanyID,
		SnapshottedAt:     time.Now(),
		Remote:            input.Remote,
		Sponsorship:       input.Sponsorship,
		RequiresCitizen:   input.RequiresCitizen,
		SecurityClearance: input.SecurityClearance,
		ExperienceYears:   input.ExperienceYears,
		TechStack:         input.TechStack,
	}
	if input.Country != "" {
		row.Country = &input.Country
	}
	if input.State != "" {
		row.State = &input.State
	}
	if input.EmployType != "" {
		row.EmployType = &input.EmployType
	}
	if input.RoleCategory != "" {
		row.RoleCategory = &input.RoleCategory
	}
	return s.db.Create(&row).Error
}

func (s *snapshotStore) UpdateSnapshotAction(id, action string) error {
	now := time.Now()
	return s.db.Model(&model.JobSnapshot{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"user_action": action,
			"actioned_at": now,
		}).Error
}
