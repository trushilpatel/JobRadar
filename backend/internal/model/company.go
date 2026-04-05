package model

import "time"

type Company struct {
	ID             string     `gorm:"primaryKey"          json:"id"`
	Name           string     `gorm:"not null"            json:"name"`
	URL            string     `gorm:"not null"            json:"url"`
	AdapterType    string     `gorm:"not null"            json:"adapter_type"`
	Active         bool       `gorm:"not null;default:true" json:"active"`
	CreatedAt      time.Time  `gorm:"not null;autoCreateTime" json:"created_at"`
	LastScanAt     *time.Time `json:"last_scan_at"`
	LastScanStatus *string    `json:"last_scan_status"`
}
