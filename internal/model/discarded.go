package model

import "time"

type Discarded struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	JobResultID string    `gorm:"not null"`
	Reason      string    `gorm:"not null"`
	DiscardedAt time.Time `gorm:"not null;autoCreateTime"`
	Note        *string

	Job Job `gorm:"foreignKey:JobResultID"`
}

func (Discarded) TableName() string {
	return "job_discarded"
}
