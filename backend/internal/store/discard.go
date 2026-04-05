package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// DiscardStore writes dismissed jobs to the discarded table.
type DiscardStore interface {
	InsertDiscard(jobResultID, reason, note string) error
}

type discardStore struct {
	db *gorm.DB
}

func NewDiscardStore(db *gorm.DB) DiscardStore {
	return &discardStore{db: db}
}

func (s *discardStore) InsertDiscard(jobResultID, reason, note string) error {
	var notePtr *string
	if note != "" {
		notePtr = &note
	}

	row := model.Discarded{
		JobResultID: jobResultID,
		Reason:      reason,
		Note:        notePtr,
	}
	return s.db.Create(&row).Error
}
