package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/trushilpatel/jobradar/config"
	_ "gorm.io/driver/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg := config.Load()

	db, err := sql.Open("pgx", cfg.DSN())
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	migrationsDir := "migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("failed to set dialect: %v", err)
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	fmt.Println("migrations applied successfully")
}
