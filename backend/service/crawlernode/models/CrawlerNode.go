package crawlernodemodel

import "time"

// CrawlerNode is the GORM model for the crawler_nodes table.
type CrawlerNode struct {
	ID          string     `gorm:"column:id;primaryKey"`
	Name        string     `gorm:"column:name;uniqueIndex;not null"`
	TokenPrefix string     `gorm:"column:token_prefix;not null"`
	TokenHash   *string    `gorm:"column:token_hash"`
	Status      string     `gorm:"column:status;not null;default:active"`
	LastSeenAt  *time.Time `gorm:"column:last_seen_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName implements the GORM Tabler interface.
func (CrawlerNode) TableName() string { return "crawler_nodes" }
