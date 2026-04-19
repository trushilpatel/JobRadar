package main

import (
	"database/sql"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	_ "gorm.io/driver/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	log := logger.Init("migrate")
	cfg := config.Load()

	db, err := sql.Open("pgx", cfg.DB.DSN())
	if err != nil {
		log.Error("failed to open db", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		log.Error("failed to connect to db", err)
		os.Exit(1)
	}

	migrationsDir := "migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Error("failed to set dialect", err)
		os.Exit(1)
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		log.Error("failed to run migrations", err)
		os.Exit(1)
	}

	log.Info("migrations applied successfully")
}
