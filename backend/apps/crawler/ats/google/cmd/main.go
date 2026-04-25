// Command google-http-test runs the HTTP-based Google Careers scraper in
// isolation. Use this to verify the scraper fetches jobs correctly before
// integrating with the pipeline.
//
// Usage:
//
//	go run ./apps/crawler/ats/google/cmd/ [flags]
//
// Flags:
//
//	-max-pages int   number of listing pages to scrape, 0 = all (default 1)
//	-max-detail int  number of detail pages to fetch, 0 = skip enrichment
//	-timeout int     overall timeout in seconds (default 180)
//	-delay-min int   min delay between listing-page fetches in ms (default 100000)
//	-delay-max int   max delay between listing-page fetches in ms (default 120000)
//	-enrich-min int  min delay between detail-page fetches in ms (default 5000)
//	-enrich-max int  max delay between detail-page fetches in ms (default 10000)
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
	maxDetail := flag.Int("max-detail", 0, "number of detail pages to fetch, 0 = skip enrichment")
	timeoutSec := flag.Int("timeout", 180, "overall timeout in seconds")
	delayMin := flag.Int("delay-min", 100000, "min delay between listing-page fetches in ms")
	delayMax := flag.Int("delay-max", 120000, "max delay between listing-page fetches in ms")
	enrichMin := flag.Int("enrich-min", 5000, "min delay between detail-page fetches in ms")
	enrichMax := flag.Int("enrich-max", 10000, "max delay between detail-page fetches in ms")
	flag.Parse()

	log := logger.Init("google-http-test")

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
	}

	log.Info("starting google http scraper test",
		logger.Int("max_pages", cfg.MaxPages),
		logger.Int("max_detail", cfg.MaxDetailJobs),
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
