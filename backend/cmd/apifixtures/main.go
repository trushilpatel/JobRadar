// Command apifixtures regenerates frontend contract-test fixtures by marshalling
// real DB rows through the exact API response DTOs. Run against a database with
// representative data; commit the output. The frontend contract test strict-parses
// these fixtures, so any backend serialization change that isn't mirrored in the
// frontend schema fails CI.
//
// Usage: DB_* env vars set, `go run ./cmd/apifixtures <out-dir>`
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: apifixtures <out-dir>")
		os.Exit(2)
	}
	outDir := os.Args[1]

	database, err := db.Open(config.DatabaseConfig{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		Name:     os.Getenv("DB_NAME"),
		SSLMode:  envOr("DB_SSLMODE", "require"),
	}, 2)
	if err != nil {
		panic(err)
	}

	var jobs []jobmodel.Job
	if err := database.Joins("Company").Order("jobs.added_at DESC").Limit(3).Find(&jobs).Error; err != nil {
		panic(err)
	}
	views := make([]jobdto.JobView, len(jobs))
	for i, j := range jobs {
		views[i] = jobdto.JobView{Job: j, UserState: jobdto.UserJobStateView{Status: "new"}}
	}
	writeJSON(filepath.Join(outDir, "jobsPage.real.json"), jobdto.PageResult{Items: views, NextCursor: ""})
}

func writeJSON(path string, v any) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
