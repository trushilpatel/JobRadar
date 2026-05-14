package application_test

import (
	"testing"

	application "github.com/trushilpatel/jobradar/service/application"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newCompatTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func TestCompat_NewStore_ReturnsRepository(t *testing.T) {
	db := newCompatTestDB(t)
	got := application.NewStore(db)
	if got == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestCompat_NewApplicationWriter_ReturnsWriter(t *testing.T) {
	db := newCompatTestDB(t)
	got := application.NewApplicationWriter(db)
	if got == nil {
		t.Fatal("NewApplicationWriter returned nil")
	}
}
