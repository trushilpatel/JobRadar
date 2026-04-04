package model

import "time"

type Company struct {
	ID             string     `gorm:"primaryKey"`
	Name           string     `gorm:"not null"`
	URL            string     `gorm:"not null"`
	AdapterType    string     `gorm:"not null"`
	Active         bool       `gorm:"not null;default:true"`
	CreatedAt      time.Time  `gorm:"not null;autoCreateTime"`
	LastScanAt     *time.Time
	LastScanStatus *string
}
