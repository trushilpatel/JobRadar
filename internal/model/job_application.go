package model

import "time"

type JobApplication struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	JobID string    `gorm:"not null"`
	Stage       string    `gorm:"not null"`
	StageAt     time.Time `gorm:"not null;autoCreateTime"`
	Note        *string

	Job Job `gorm:"foreignKey:JobResultID"`
}
