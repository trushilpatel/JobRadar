package usermodel

import "time"

// UserPreference is the GORM struct for the user_preferences table. The Data
// column holds the serialised UserConfig as a JSONB blob. Using []byte keeps
// GORM from double-encoding the value; the repository layer handles
// json.Marshal / json.Unmarshal explicitly.
type UserPreference struct {
	UserID    string    `gorm:"column:user_id;primaryKey"`
	Data      []byte    `gorm:"column:data;type:jsonb"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserPreference) TableName() string { return "user_preferences" }
