// Command google-http-test runs the HTTP-based Google Careers scraper in
// isolation. Use this to verify the scraper fetches listing pages correctly
// before integrating with the pipeline.
//
// Usage:
//
//	go run ./apps/crawler/ats/google/cmd/ [flags]
//
// Flags:
//
//	-max-pages int  number of listing pages to scrape, 0 = all (default 1)
//	-timeout int    overall timeout in seconds (default 180)
//	-delay-min int  min delay between listing-page fetches in ms (default 20000)
//	-delay-max int  max delay between listing-page fetches in ms (default 40000)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/common/logger"
)

func main() {
	maxPages := flag.Int("max-pages", 1, "number of listing pages to scrape, 0 = all")
	timeoutSec := flag.Int("timeout", 180, "overall timeout in seconds")
	delayMin := flag.Int("delay-min", 20000, "min delay between listing-page fetches in ms")
	delayMax := flag.Int("delay-max", 40000, "max delay between listing-page fetches in ms")
	flag.Parse()

	log := logger.Init("google-http-test")

	cfg := google.Config{
		MinDelayMs: *delayMin,
		MaxDelayMs: *delayMax,
		TimeoutMs:  *timeoutSec * 1000,
		MaxPages:   *maxPages,
	}

	log.Info("starting google http scraper test",
		logger.Int("max_pages", cfg.MaxPages),
		logger.Int("timeout_sec", *timeoutSec),
		logger.Int("delay_min_ms", cfg.MinDelayMs),
		logger.Int("delay_max_ms", cfg.MaxDelayMs),
	)

	scraper := google.NewGoogleAdapter(cfg, log)

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
