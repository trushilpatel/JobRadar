package scanmodel

import "time"

// AdapterConfig holds per-adapter scan configuration stored in the adapter_configs table.
// One row per adapter type; scan intervals and inter-company delays are controlled here.
type AdapterConfig struct {
	AdapterType         string     `gorm:"primaryKey"`
	ScanIntervalMinutes int        `gorm:"not null;default:1440"`
	InterCompanyDelayS  int        `gorm:"not null;default:5"`
	LastScheduledAt     *time.Time `json:"last_scheduled_at"`
	CreatedAt           time.Time  `gorm:"not null;autoCreateTime"`
	UpdatedAt           time.Time  `gorm:"not null;autoUpdateTime"`
}

func (AdapterConfig) TableName() string {
	return "adapter_configs"
}
