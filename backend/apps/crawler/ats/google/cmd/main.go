// Command google-rod-test runs the rod-based Google Careers scraper in isolation.
// Use this to verify the scraper fetches jobs correctly before integrating with the pipeline.
//
// Usage:
//
//	go run ./apps/crawler/ats/google/cmd/ [flags]
//
// Flags:
//
//	-max-pages int   number of listing pages to scrape, 0 = all (default 1)
//	-timeout  int    overall timeout in seconds (default 120)
//	-delay-min int   min delay between page navigations in ms (default 5000)
//	-delay-max int   max delay between page navigations in ms (default 10000)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/common/logger"
)

func main() {
	maxPages := flag.Int("max-pages", 1, "number of listing pages to scrape, 0 = all")
	maxDetail := flag.Int("max-detail", 0, "number of detail pages to visit, 0 = skip enrichment")
	timeoutSec := flag.Int("timeout", 180, "overall timeout in seconds")
	delayMin := flag.Int("delay-min", 5000, "min delay between page navigations in ms")
	delayMax := flag.Int("delay-max", 10000, "max delay between page navigations in ms")
	enrichMin := flag.Int("enrich-min", 3000, "min delay between detail page visits in ms")
	enrichMax := flag.Int("enrich-max", 5000, "max delay between detail page visits in ms")
	defaultProfile := filepath.Join(os.Getenv("HOME"), ".jobradar-chrome")
	profileDir := flag.String("profile-dir", defaultProfile, "persistent Chrome profile directory (empty = fresh profile)")
	flag.Parse()

	log := logger.Init("google-rod-test")

	// EnrichMaxMs == 0 signals the adapter to skip detail visits entirely.
	enrichMaxVal := 0
	if *maxDetail > 0 {
		enrichMaxVal = *enrichMax
	}

	cfg := google.Config{
		MinDelayMs:    *delayMin,
		MaxDelayMs:    *delayMax,
		TimeoutMs:     *timeoutSec * 1000,
		MaxPages:      *maxPages,
		EnrichMinMs:   *enrichMin,
		EnrichMaxMs:   enrichMaxVal,
		MaxDetailJobs: *maxDetail,
		ProfileDir:    *profileDir,
	}

	log.Info("starting rod scraper test",
		logger.Int("max_pages", cfg.MaxPages),
		logger.Int("timeout_sec", *timeoutSec),
		logger.Int("delay_min_ms", cfg.MinDelayMs),
		logger.Int("delay_max_ms", cfg.MaxDelayMs),
	)

	scraper := google.NewRodGoogle(cfg, log)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
	defer cancel()

	resp, err := scraper.FetchJobs(ctx, "google")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, err := json.MarshalIndent(resp.Jobs, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(out))
	fmt.Fprintf(os.Stderr, "\n--- total jobs fetched: %d ---\n", len(resp.Jobs))
}
